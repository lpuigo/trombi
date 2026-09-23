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
cmd/trombi/main.go                       CLI : flags, wiring, formatage du rapport (image unique)

service/
  process.go                             orchestration image unique : ProcessImage, ReframeImage,
                                          Cropper
  batch.go                               orchestration lot : ProcessBatch, ProcessItem, ReframeItem,
                                          ImageLoader

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
  item.go                                Item, NewItem
  batch.go                               Batch, NewBatch, Batch.Exportable
  grid.go                                GridLayout, DefaultGridLayout
  framing_test.go                        tests de ComputeCropBox / FitWithinBounds

infra/facedetect/pigodetect/detector.go  Detector (implémente portrait.FaceDetector via Pigo)
infra/imageio/
  decode.go                              Load (décodage fichier → SourceImage)
  exif.go                                readJPEGOrientation (parsing EXIF minimal)
  orientation.go                         applyOrientation (réorientation en mémoire)
  crop.go                                Cropper (implémente service.Cropper)
  loader.go                              Loader (implémente service.ImageLoader)
  list.go                                ListDir (liste les images d'un dossier)

infra/pdfexport/
  diagnostic.go                          Exporter (rend un Diagnostic en PDF, image unique)
  grid.go                                GridExporter (rend un Batch en PDF grille L×C, paginé)
```

Seule une CLI existe pour l'instant (pas encore de serveur HTTP/GUI, cf. spec §10.2), et elle ne
couvre encore que le pipeline image unique (`service.ProcessImage`) — le pipeline par lot
(`service.ProcessBatch` et consorts) existe au niveau domain/service/infra mais n'est pas encore
câblé à un point d'entrée utilisateur (ni CLI, ni GUI) ; voir §3.

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
- **`Status`** : `StatusPending` (valeur zéro — aucun traitement encore effectué, cf. `Item` non
  traité dans un `Batch` fraîchement créé), `StatusSuccess`, `StatusWarning`, `StatusFailure`.
- **`ProcessingResult`** — résultat du traitement d'une image, agrégat consommé par les exporteurs
  et par le rapport de lot.
  - **Attributs** : `Status`, `SourceImage`, `DetectedFaces []DetectedFace`,
    `SelectedFace DetectedFace`, `Diagnostic`, `Warnings []Warning`, `FailureReason error`.

#### `Item` (`item.go`) — unité de traitement d'un lot
Regroupe tout ce qu'il faut pour traiter — et retraiter — une image de façon autonome au sein d'un
lot : son chemin source, son nom d'affichage (légende de l'export grille), les paramètres de
cadrage qui lui sont propres, et le dernier résultat de traitement produit avec ces paramètres.

- **Attributs** : `SourcePath string`, `Name string` (légende éditable, initialisée au nom de
  fichier sans extension), `Spec FramingSpec` (propre à l'item — peut diverger du `DefaultSpec` du
  `Batch` une fois modifiée par l'utilisateur), `Result ProcessingResult`.
- **Constructeur** : `NewItem(path string, spec FramingSpec) Item` — item non traité
  (`Result.Status == StatusPending`), `Name` dérivé de `path`.

#### `Batch` (`batch.go`) — lot de traitement
Ensemble d'`Item`, construit à partir de tous les fichiers image valides d'un dossier source.

- **Attributs** : `DefaultSpec FramingSpec`, `Items []Item`.
- **Constructeur** : `NewBatch(paths []string, defaultSpec FramingSpec) Batch` — un `Item` non
  traité par chemin, tous initialisés avec `defaultSpec` ; ne fait aucun I/O (les chemins sont déjà
  fournis par l'appelant — voir `imageio.ListDir` côté infra).
- **Verbe métier** : `Exportable() []Item` — filtre les items dont le dernier traitement a produit
  un portrait (`StatusSuccess` ou `StatusWarning`), dans l'ordre du lot ; c'est cette liste que
  consomme `pdfexport.GridExporter`.

#### `GridLayout` (`grid.go`) — configuration de l'export grille
- **Attributs** : `Rows int`, `Cols int` (L lignes × C colonnes par page).
- **Constructeur** : `DefaultGridLayout() GridLayout` — 4×3, soit 12 portraits par page A4.

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

`ProcessImage` et `ReframeImage` partagent leur seconde moitié (calcul de la box de crop, clamping,
crop/resize) via la fonction non exportée `frameAndCrop` — `ReframeImage` saute juste la détection
en repartant des `faces`/`selected` déjà connus.

- **`ReframeImage(cropper, spec, src, faces, selected) ProcessingResult`** — recalcule la box de
  crop et le portrait pour un visage déjà détecté, sous un nouveau `FramingSpec`, **sans relancer
  la détection**. C'est l'opération appelée quand l'utilisateur modifie les marges/ratio d'un
  `Item` dans l'IHM : la détection (coûteuse) n'a pas besoin d'être refaite, seul le cadrage change.

#### `ImageLoader` (interface, `batch.go`)
Interface possédée par la couche service (même logique que `Cropper`) pour ne pas faire dépendre le
pipeline de lot de `infra/imageio` directement.

- **Méthode** : `Load(path string) (SourceImage, error)`.
- **Implémentation** : `imageio.Loader` (infra), simple adaptateur autour de la fonction
  `imageio.Load` existante.

#### `ProcessBatch` / `ProcessItem` / `ReframeItem` (fonctions, `batch.go`) — orchestration du lot
```
ProcessBatch(detector FaceDetector, cropper Cropper, loader ImageLoader, b Batch) Batch
ProcessItem(detector FaceDetector, cropper Cropper, loader ImageLoader, item Item) Item
ReframeItem(cropper Cropper, spec FramingSpec, item Item) (Item, error)
```

- **`ProcessBatch`** applique `ProcessItem` à chaque `Item` du lot et retourne un nouveau `Batch`
  (ne modifie pas `b`) — une image en échec de chargement/détection/cadrage ne stoppe jamais le
  reste du lot (spec §8/§11), l'échec est isolé dans le `Result` de cet `Item` seul.
- **`ProcessItem`** charge l'image source via `loader.Load` puis délègue à
  `service.ProcessImage` ; un échec de chargement produit directement un
  `ProcessingResult{Status: StatusFailure}` sans appeler le détecteur.
- **`ReframeItem`** est l'opération de retraitement léger déclenchée par l'IHM (cf. `ReframeImage`
  ci-dessus) : échoue avec `portrait.ErrNoFaceDetected` si l'item n'a jamais eu de détection
  réussie à réutiliser, sinon met à jour `item.Spec` et recalcule `item.Result` via
  `ReframeImage`.

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

#### `imageio.Loader` (`infra/imageio/loader.go`)
Adaptateur sans état qui implémente `service.ImageLoader` en délégant à la fonction `Load`
ci-dessus — permet l'injection de dépendance dans `service.ProcessBatch`/`ProcessItem`.

- **Méthode** : `Load(path string) (SourceImage, error)`.

#### `imageio.ListDir` (`infra/imageio/list.go`)
- **Signature** : `ListDir(dir string) ([]string, error)`.
- **Comportement** : liste les fichiers d'un dossier (non récursif) dont l'extension correspond à
  un format supporté (`.jpg`, `.jpeg`, `.png`), triés par nom pour un ordre de traitement du lot
  stable et prévisible. C'est le point d'entrée I/O attendu en amont de `portrait.NewBatch`.

#### `pdfexport.Exporter` (`infra/pdfexport/diagnostic.go`)
Rend un `portrait.Diagnostic` en PDF de diagnostic une page (spec §9.4), via `go-pdf/fpdf`.

- **Méthode** : `Export(d Diagnostic, outputPath string) error` — dispose l'image source annotée
  des deux bounding boxes (rouge = `FaceBox`, vert = `CropBox`) en haut de page, et le portrait
  normalisé en dessous.

#### `pdfexport.GridExporter` (`infra/pdfexport/grid.go`)
Rend les portraits d'un `Batch` en PDF final du trombinoscope : une grille paginée de
`GridLayout.Rows` × `GridLayout.Cols` portraits par page, via `go-pdf/fpdf` — le livrable final,
distinct du PDF de diagnostic par image.

- **Méthode** : `Export(items []Item, layout GridLayout, outputPath string) error` — attend en
  entrée une liste déjà filtrée (typiquement `Batch.Exportable()`) ; dessine, pour chaque `Item`,
  son portrait centré dans sa cellule avec `Item.Name` en légende dessous, et ajoute une nouvelle
  page dès qu'une page est pleine.

### 2.5 `cmd/trombi` — CLI

`main.run(args []string) int` parse les options de cadrage (marges, ratio, résolution, chemin de
sortie), construit `pigodetect.Detector` et `imageio.Cropper`, appelle `service.ProcessImage`, puis
délègue le rendu à `pdfexport.Exporter`. Les fonctions `describeFailure`/`describeWarning`
traduisent les erreurs et `Warning` du domaine en messages utilisateur (français) — aucune règle
métier n'est dupliquée ici.

## 3. Points notables pour la suite

- Le pipeline par lot (`portrait.Batch`/`Item`, `service.ProcessBatch`/`ProcessItem`/`ReframeItem`,
  `pdfexport.GridExporter`) existe désormais au niveau domain/service/infra, mais **n'est pas
  encore câblé à un point d'entrée utilisateur** : ni la CLI (qui ne traite qu'un fichier à la
  fois, cf. `cmd/trombi/main.go`) ni une GUI (spec §10.2, toujours pas démarrée) ne l'exposent
  encore. Câblage CLI (choix fichier vs dossier, flags de layout de grille, rapport console par
  lot) à faire dans une prochaine étape.
- Pas encore d'export JPEG "portrait seul" (spec §9.4) : seuls le PDF de diagnostic (image unique)
  et le PDF grille (lot) sont câblés côté infra.
- `domain/portrait` reste indépendant de tout I/O et testable avec de simples structs
  (`framing_test.go` couvre `ComputeCropBox`/`FitWithinBounds` sans décoder d'image), conformément
  à `design-rules.md` — `Item`/`Batch`/`GridLayout` suivent la même règle (`NewBatch` ne fait
  aucun I/O : les chemins lui sont fournis déjà listés par `imageio.ListDir`).
