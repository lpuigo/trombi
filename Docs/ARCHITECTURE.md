# Architecture logicielle — trombi

> Spécification fonctionnelle associée : [`SPEC_trombinoscope.md`](./SPEC_trombinoscope.md).
> Règles de design appliquées au découpage en couches : [`design-rules.md`](./design-rules.md).

Ce document décrit l'état actuel du code (prototype CLI, cf. `Docs/SPEC_trombinoscope.md` §13.4) :
son organisation en couches, les packages, et les types qui portent la logique métier avec leurs
principales méthodes et les liens entre eux.

## 1. Organisation du code

Le code respecte le découpage en couches imposé par `design-rules.md`, décliné pour ce pipeline
image (`CLAUDE.md`, section Architecture) :

```
cmd/trombi/main.go              → CLI (handler) : parse les flags, appelle service.ProcessImage,
                                    formate le rapport et les erreurs — aucune logique métier
        ↓
service/process.go              → orchestration du pipeline : détection → sélection → cadrage →
                                    crop/export → agrégation du résultat
        ↓
domain/portrait/                → modèle et règles métier pures (aucun I/O)
        ↓
infra/facedetect/pigodetect/    → détecteur de visage (Pigo)
infra/imageio/                  → décodage image, orientation EXIF, crop/resize
infra/pdfexport/                → export PDF de diagnostic
```

Arborescence des fichiers :

```
cmd/trombi/main.go                       CLI : flags, wiring, formatage du rapport

service/process.go                       orchestration : service.ProcessImage, service.Cropper

domain/portrait/
  image.go                               SourceImage
  boundingbox.go                         BoundingBox
  face.go                                DetectedFace
  detector.go                            FaceDetector (port)
  selection.go                           SelectPrimaryFace
  framing.go                             FramingSpec, AspectRatio, Resolution, ComputeCropBox
  clamp.go                               FitWithinBounds (politique de clamping bord d'image)
  diagnostic.go                          NormalizedPortrait, Diagnostic
  result.go                              Status, ProcessingResult
  warning.go                             WarningKind, Warning
  framing_test.go                        tests de ComputeCropBox / FitWithinBounds

infra/facedetect/pigodetect/detector.go  Detector (implémente portrait.FaceDetector via Pigo)
infra/imageio/
  decode.go                              Load (décodage fichier → SourceImage)
  exif.go                                readJPEGOrientation (parsing EXIF minimal)
  orientation.go                         applyOrientation (réorientation en mémoire)
  crop.go                                Cropper (implémente service.Cropper)

infra/pdfexport/diagnostic.go            Exporter (rend un Diagnostic en PDF)
```

Seule une CLI existe pour l'instant (pas encore de serveur HTTP/GUI, cf. spec §10.2) ; l'unique
appelant de `service.ProcessImage` est `cmd/trombi/main.go`.

## 2. Classes/types portant la logique métier

### 2.1 Vue d'ensemble des liens

```
main.run()
   │  construit
   ▼
pigodetect.Detector ──implémente──▶ portrait.FaceDetector ◀──dépend de── service.ProcessImage
imageio.Cropper      ──implémente──▶ service.Cropper       ◀──dépend de── service.ProcessImage
portrait.FramingSpec ───────────────────────────────────────consommé par─ service.ProcessImage
                                                                    │
                                                                    ▼ orchestre
                                              portrait.SelectPrimaryFace
                                              portrait.ComputeCropBox
                                              portrait.FitWithinBounds
                                                                    │
                                                                    ▼ produit
                                              portrait.ProcessingResult { Diagnostic }
                                                                    │
                                                                    ▼ consommé par
                                              pdfexport.Exporter.Export
```

`service.ProcessImage` est le seul point d'orchestration : il ne connaît que les deux interfaces
qu'il consomme (`portrait.FaceDetector`, `service.Cropper`) et les fonctions pures de `domain/portrait`
— aucune dépendance vers `infra/*` ni vers Pigo/fpdf/x-image-draw.

### 2.2 `domain/portrait` — modèle et règles métier

#### `SourceImage` (`image.go`)
Image source décodée, dans son propre repère pixel pleine résolution.

- **Attributs** : `Pixels image.Image`, `Width int`, `Height int`.
- **Constructeur** : `NewSourceImage(pixels) SourceImage` — dérive `Width`/`Height` de `pixels.Bounds()`.

#### `BoundingBox` (`boundingbox.go`)
Rectangle en pixels dans le repère de l'image source. Type unique, réutilisé pour la bounding box
native d'un visage comme pour la box de crop (pas de type séparé — cf. `CLAUDE.md`).

- **Attributs** : encapsule `image.Rectangle` (coins `Min`/`Max`).
- **Constructeur** : `NewBoundingBox(x0, y0, x1, y1) BoundingBox`.
- **Verbes métier** :
  - `Width()`, `Height()`, `Area()` — dimensions dérivées.
  - `CenterX()`, `CenterY()` — centre du rectangle.
  - `Translated(dx, dy) BoundingBox` — décalage à taille inchangée.
  - `ScaledAboutCenter(factor) BoundingBox` — réduction/agrandissement autour du centre, en
    préservant le ratio d'origine du rectangle.
  - `FitsWithin(width, height) bool` — teste l'inclusion dans un cadre `0,0-(width,height)`.
  - `Contains(other) bool` — teste l'inclusion d'un autre `BoundingBox`.

#### `DetectedFace` (`face.go`)
Un visage détecté, dans le repère de la `SourceImage` où il a été trouvé.

- **Attributs** : `Box BoundingBox`, `Score float32` (score brut du détecteur, **non normalisé** —
  ne jamais le comparer à un seuil fixe sans calibration, cf. spec §7/§12).

#### `FaceDetector` (`detector.go`) — port du domaine
Unique interface du domaine (cf. `CLAUDE.md` : justifiée par la spec §7/§12 qui signale Pigo comme
un choix révisable).

- **Méthode** : `Detect(SourceImage) ([]DetectedFace, error)`.
- **Implémentation** : `pigodetect.Detector` (infra).

#### `SelectPrimaryFace` (`selection.go`) — fonction pure de sélection
Règle de sélection en cas d'ambiguïté (spec §8) : suppose que `faces` est déjà dédupliqué par IoU
en amont (`ClusterDetections`, côté `pigodetect.Detector`).

- **Signature** : `SelectPrimaryFace(faces []DetectedFace) (DetectedFace, []Warning, error)`.
- **Comportement** : sélectionne le visage de plus grande aire ; retourne
  `ErrNoFaceDetected` si `faces` est vide ; ajoute un `Warning{Kind: WarningMultipleFacesDetected}`
  si plus d'un visage survit.

#### `FramingSpec` / `AspectRatio` / `Resolution` (`framing.go`) — configuration de cadrage
`FramingSpec` est la configuration validée du cadrage ; `TopMargin`/`BottomMargin`/`Ratio` sont la
source de vérité (spec §9.2), la marge horizontale et la position verticale du visage étant des
valeurs dérivées, non stockées.

- **Attributs de `FramingSpec`** : `TopMargin float64`, `BottomMargin float64` (fractions de la
  hauteur du visage), `Ratio AspectRatio`, `Output Resolution`.
- **`AspectRatio{Width, Height float64}`** — verbe : `Value() float64` (ratio largeur/hauteur).
- **`Resolution{Width, Height int}`** — dimensions pixel de l'export.
- **Constructeur** : `DefaultFramingSpec() FramingSpec` — valeurs par défaut de la spec §9.2
  (marges 30 %/47 %, ratio 3:4, résolution 600×800).
- **Verbe métier central** : `ComputeCropBox(face BoundingBox, spec FramingSpec) BoundingBox` —
  calcule la box de crop : hauteur dérivée des marges + hauteur du visage, largeur dérivée de la
  hauteur × ratio, centrée horizontalement sur le visage.

#### `FitWithinBounds` (`clamp.go`) — politique de clamping bord d'image
Implémente la politique de bord d'image de la spec §8 : translation d'abord, réduction ensuite,
échec en dernier recours.

- **Signature** :
  `FitWithinBounds(box, faceBox BoundingBox, sourceWidth, sourceHeight int) (BoundingBox, []Warning, error)`.
- **Comportement** : si `box` sort du cadre source, réduit `box` (autour de son centre, ratio
  préservé) puis la translate pour rentrer dans le cadre ; ajoute un
  `Warning{Kind: WarningCropClamped}` si un ajustement a eu lieu.
- **Erreurs** : `ErrFaceExceedsSourceBounds` (la bounding box native du visage dépasse déjà le
  cadre — cas pathologique) ; `ErrCropExcludesFace` (même réduite/translatée, la box de crop ne
  contient plus entièrement le visage).

#### `NormalizedPortrait` / `Diagnostic` (`diagnostic.go`) — view models d'export
- **`NormalizedPortrait{Pixels image.Image, Width, Height int}`** — portrait final recadré et
  redimensionné, prêt à exporter.
- **`Diagnostic{Source SourceImage, FaceBox, CropBox BoundingBox, Portrait NormalizedPortrait}`**
  — view model partagé par les deux méthodes d'export (JPEG, PDF) et la future UI HTML (spec §9.4) :
  porte tout ce dont un renderer a besoin pour dessiner l'image source annotée des deux bounding
  boxes, à n'importe quelle échelle, sans avoir à redécoder l'image.

#### `Warning` / `WarningKind` (`warning.go`)
- **`WarningKind`** : `WarningMultipleFacesDetected`, `WarningCropClamped`.
- **`Warning{Kind WarningKind, FaceCount int}`** (`FaceCount` renseigné uniquement pour
  `WarningMultipleFacesDetected`). Le domaine se contente d'enregistrer *quoi* ; la mise en forme
  du message utilisateur est laissée à la couche présentation (`main.describeWarning`).

#### `Status` / `ProcessingResult` (`result.go`) — agrégat
- **`Status`** : `StatusSuccess`, `StatusWarning`, `StatusFailure`.
- **`ProcessingResult`** — résultat du traitement d'une image, agrégat consommé par les exporteurs
  et par le futur rapport de lot.
  - **Attributs** : `Status`, `SourceImage`, `DetectedFaces []DetectedFace`,
    `SelectedFace DetectedFace`, `Diagnostic`, `Warnings []Warning`, `FailureReason error`.

### 2.3 `service` — orchestration

#### `Cropper` (interface, `process.go`)
Interface possédée par la couche service (pas par le domaine) car elle délègue le rééchantillonnage
de pixels à une bibliothèque d'imagerie dont le domaine n'a pas à connaître les détails.

- **Méthode** : `Crop(src SourceImage, box BoundingBox, out Resolution) (NormalizedPortrait, error)`.
- **Implémentation** : `imageio.Cropper` (infra).

#### `ProcessImage` (fonction, `process.go`)
Le seul verbe de la couche service — orchestre le pipeline complet pour une image :

```
ProcessImage(detector FaceDetector, cropper Cropper, spec FramingSpec, src SourceImage) ProcessingResult
```

Séquence : `detector.Detect` → `portrait.SelectPrimaryFace` → `portrait.ComputeCropBox` →
`portrait.FitWithinBounds` → `cropper.Crop` → assemblage du `ProcessingResult` (avec
`Diagnostic`). Toute erreur à une étape produit un `ProcessingResult{Status: StatusFailure}` sans
interrompre l'appelant (isolation des échecs, spec §8/§11) ; les avertissements des étapes de
sélection et de clamping sont cumulés dans `Warnings`.

### 2.4 Implémentations infra

#### `pigodetect.Detector` (`infra/facedetect/pigodetect/detector.go`)
Implémente `portrait.FaceDetector` avec [Pigo](https://github.com/esimov/pigo) (Go pur, cascade
`facefinder` embarquée via `go:embed`).

- **Attributs** : paramètres de cascade Pigo (`minSize`, `maxSize`, `shiftFactor`, `scaleFactor`,
  `iouThreshold`), alignés sur les valeurs par défaut de la CLI de référence de Pigo.
- **Constructeur** : `NewDetector() (*Detector, error)` — dépaquette la cascade embarquée.
- **Méthode** : `Detect(SourceImage) ([]DetectedFace, error)` — exécute la cascade puis
  `ClusterDetections` (fusion IoU, spec §7/§8) avant de convertir chaque détection Pigo (carrée,
  `Row/Col/Scale`) en `portrait.BoundingBox`.

#### `imageio.Cropper` (`infra/imageio/crop.go`)
Implémente `service.Cropper` via `golang.org/x/image/draw` (filtre `CatmullRom`, spec §9.4).

- **Méthode** : `Crop(src, box, out) (NormalizedPortrait, error)` — intersecte `box` avec les
  bornes de l'image source, puis redimensionne directement vers la résolution de sortie.

#### `imageio.Load` et helpers d'orientation (`decode.go`, `exif.go`, `orientation.go`)
- **`Load(path string) (SourceImage, error)`** — décode un fichier (JPEG/PNG via la stdlib), lit
  le tag EXIF `Orientation` pour un JPEG (`readJPEGOrientation`, parsing TIFF minimal sans
  dépendance externe) et réoriente l'image en mémoire si nécessaire (`applyOrientation`) avant de
  construire le `SourceImage` (spec §7/§8).

#### `pdfexport.Exporter` (`infra/pdfexport/diagnostic.go`)
Rend un `portrait.Diagnostic` en PDF de diagnostic une page (spec §9.4), via `go-pdf/fpdf`.

- **Méthode** : `Export(d Diagnostic, outputPath string) error` — dispose l'image source annotée
  des deux bounding boxes (rouge = `FaceBox`, vert = `CropBox`) en haut de page, et le portrait
  normalisé en dessous.

### 2.5 `cmd/trombi` — CLI

`main.run(args []string) int` parse les options de cadrage (marges, ratio, résolution, chemin de
sortie), construit `pigodetect.Detector` et `imageio.Cropper`, appelle `service.ProcessImage`, puis
délègue le rendu à `pdfexport.Exporter`. Les fonctions `describeFailure`/`describeWarning`
traduisent les erreurs et `Warning` du domaine en messages utilisateur (français) — aucune règle
métier n'est dupliquée ici.

## 3. Points notables pour la suite

- Pas encore de traitement par lot (dossier entier) ni de rapport agrégé multi-fichiers — le
  pipeline actuel traite une image à la fois (spec §13.4, prototype).
- Pas encore d'export JPEG "portrait seul" (spec §9.4) ni de serveur HTTP/GUI (spec §10.2) :
  seul l'export PDF de diagnostic est câblé dans la CLI.
- `domain/portrait` reste indépendant de tout I/O et testable avec de simples structs
  (`framing_test.go` couvre `ComputeCropBox`/`FitWithinBounds` sans décoder d'image), conformément
  à `design-rules.md`.
