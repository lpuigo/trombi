# CLAUDE.md

Ce fichier fournit des instructions à Claude Code (claude.ai/code) pour travailler dans ce dépôt.

## Langue

- **Documents de spécification et de design technique** (`Docs/*.md`, et plus généralement toute
  documentation de cadrage) : rédigés **en français**.
- **Code, identifiants et commentaires** : rédigés **en anglais**, y compris le vocabulaire propre
  au domaine fonctionnel de l'application (ex. `face`, `crop`, `bounding box`, `report`, `margin`,
  `detector`, plutôt que leurs équivalents français). Ne pas traduire les termes métier en anglais
  dans le code sous prétexte que la spec est en français — c'est l'inverse : la spec reste en
  français, son vocabulaire est traduit en anglais au moment de nommer les types/fonctions/packages.

## État du projet

Projet greenfield : seuls `go.mod` (module `trombi`, Go 1.24) et les deux documents de cadrage
ci-dessous existent. Aucun code source, aucun test, aucun outillage de build pour l'instant. Lors
de l'ajout des premiers packages, mettre en place dès le départ la structure en couches décrite
ici plutôt que de la laisser émerger organiquement — voir `Docs/design-rules.md` (importé
ci-dessous), dont l'objectif explicite est d'être appliqué *avant* la première ligne de logique
métier de ce projet.

@Docs/design-rules.md

@Docs/SPEC_trombinoscope.md

## Objectif de l'outil

`trombi` transforme un dossier de photos de portraits individuels en un jeu de portraits uniformes
pour un trombinoscope d'équipe : détection du visage sur chaque image source, calcul d'un cadrage
à partir de marges/ratio/résolution de sortie configurables, export des portraits recadrés et d'un
rapport de traitement (succès / échec par fichier). Voir `Docs/SPEC_trombinoscope.md` pour la
spécification fonctionnelle complète — points clés qui contraignent les choix d'implémentation :

- **Traitement 100% local** — aucune image ni donnée de chemin ne doit jamais quitter la machine.
  Ceci exclut toute API de détection de visage basée cloud (Rekognition, Google Vision, Azure
  Face, etc.).
- **Go pur, sans CGo** — le développement se fait sous Windows mais l'outil doit pouvoir être
  cross-compilé proprement vers macOS/Linux depuis un poste Windows, donc éviter les bindings CGo
  et les toolkits natifs (c'est la raison du choix de [Pigo](https://github.com/esimov/pigo) comme
  détecteur de visage, et pourquoi les toolkits GUI natifs comme Fyne/Wails sont écartés a priori —
  voir §10.2 de la spec).
- **Deux interfaces pour un seul cœur** : une CLI (batch, scriptable, code de retour + rapport) et
  une GUI. La piste GUI privilégiée par la spec est un serveur HTTP local (stdlib) servant une UI
  web dans le navigateur, précisément parce qu'elle n'ajoute aucune dépendance CGo/toolkit — les
  deux interfaces doivent être de simples appelants de la couche service/domain, jamais des
  réimplémentations du pipeline.
- **L'échec d'une image ne doit jamais interrompre le traitement du lot** — les échecs sont isolés
  et consignés dans le rapport (aucun visage détecté, plusieurs visages détectés, format
  illisible/non supporté, cadrage sortant du cadre source).
- Les paramètres de cadrage (ratio de sortie, résolution de sortie, marges horizontale/haute/basse
  en % de la taille du visage, position verticale du visage dans le cadre, format/qualité de
  sortie) sont configurables par l'utilisateur via options CLI et/ou GUI et/ou fichier de
  configuration, avec des valeurs par défaut raisonnables — à traiter comme de la configuration au
  niveau domain, pas comme des constantes en dur.

## Architecture (à mettre en place au fur et à mesure du code)

Respecter strictement le découpage en couches imposé par `Docs/design-rules.md`, appliqué à ce
pipeline (voir la section 6 de la spec pour le détail des étapes du pipeline) :

```
cli / webserver (handler)   → parse l'entrée (flags/requête HTTP), appelle la couche service, affiche/sert le rapport — aucune logique de détection ou de cadrage ici
        ↓
service (orchestration)     → exécute le pipeline par image : détection → résolution d'ambiguïté → calcul du rectangle de cadrage → redimensionnement/export → agrégation du rapport
        ↓
domain (model)               → struct de configuration de cadrage, calcul du rectangle de cadrage à partir d'une bounding box de visage, types d'entrées de rapport — logique pure, aucun I/O fichier/réseau
        ↓
infra (detect / image I/O)  → détecteur de visage basé sur Pigo derrière une interface étroite, décodage/encodage/redimensionnement d'image, lecture/écriture disque
```

- Le **détecteur de visage doit être placé derrière une interface possédée par la couche
  domain/service** (ex. une interface `FaceDetector` avec une méthode du type
  `Detect(image) ([]FaceBox, error)`), l'implémentation basée sur Pigo vivant dans infra. Ce n'est
  pas de l'abstraction prématurée — la spec signale explicitement le choix du détecteur comme un
  point ouvert susceptible d'être remplacé par une alternative CGo (gocv) si Pigo se révèle
  insuffisant sur des photos réelles (§7, §12). Ne pas laisser les types de Pigo fuiter dans
  domain/service.
- Le **calcul du rectangle de cadrage (bounding box du visage + marges/ratio → rectangle de sortie,
  y compris le clamping en bord d'image) doit vivre dans la couche domain** et doit être testable
  avec de simples structs — sans décodage d'image nécessaire pour le tester.
- **CLI et GUI sont des interfaces alternatives au niveau handler** au-dessus du même service — ne
  pas dupliquer la logique du pipeline entre les deux.
- Le traitement par lot doit **isoler les échecs par fichier** (aucun visage / plusieurs visages /
  format invalide) dans un type de rapport produit par la couche service, plutôt que de laisser
  remonter des erreurs brutes qui interrompraient le traitement.

## Modèle de domaine (portrait framing)

Issu d'une analyse DDD du traitement d'une image (détection, bounding box, cadrage) menée le
2026-09-03 — voir les décisions correspondantes dans `Docs/SPEC_trombinoscope.md` §7/§8/§9/§12.
Volontairement resserré (§5/§6 de `design-rules.md`, YAGNI) : une seule interface (`FaceDetector`),
le reste en fonctions/structs simples tant que rien dans la spec ne signale un point de variation.

**Value Objects** (package `domain`, immutables) :
- `SourceImage` — pixels décodés + dimensions.
- `BoundingBox` — rectangle en pixels dans le repère de l'image source pleine résolution ; un seul
  type, réutilisé aussi bien pour la box native du visage que pour la box de crop (pas de type
  `CropBoundingBox` séparé).
- `DetectedFace{ Box BoundingBox; Score float32 }` — `Score` est le `Q` brut de Pigo, un score de
  classifieur **non normalisé** (pas une probabilité [0,1]) : ne jamais lui appliquer un seuil fixe
  sans calibration préalable sur un échantillon réel.
- `FramingSpec` — configuration de cadrage validée à la construction (marges haute/basse, ratio,
  résolution, format/qualité d'export). La marge horizontale et la position verticale du visage
  sont des valeurs **dérivées** de ce spec, pas des champs indépendants (voir §9.2 de la spec pour
  le pourquoi : sur-détermination du rectangle de cadrage sinon).
- `PortraitDiagnostic{ SourceWidth, SourceHeight int; FaceBox, CropBox BoundingBox; NormalizedPortrait }`
  — le view-model partagé par les deux méthodes d'export (JPEG, PDF) et par la future UI HTML ;
  porter les dimensions source ici est ce qui permet à n'importe quel renderer de recalculer
  l'échelle d'affichage sans redécoder l'image.
- `NormalizedPortrait` — pixels recadrés et redimensionnés à la résolution de sortie.

**Aggregate** : `PortraitProcessingResult{ Status; SourceImage; DetectedFaces []; SelectedFace;
Diagnostic; NormalizedPortrait; Warnings []; FailureReason }` — résultat du traitement d'une image,
consommé par le futur rapport de lot et par les deux exporteurs.

**Domain services — fonctions pures, pas d'interfaces** (une seule implémentation attendue pour
chacune ; ne pas les transformer en interfaces/stratégies tant que la spec ne signale pas un besoin
de variation, cf. §6 anti-pattern « factorisation prématurée ») :
- `SelectPrimaryFace(faces) (DetectedFace, warnings, error)` — fusion IoU des détections
  qui se chevauchent d'abord (`ClusterDetections` côté infra Pigo), puis sélection par aire max.
- `ComputeCropBox(faceBox, spec) BoundingBox`
- `FitWithinBounds(box, sourceW, sourceH) (BoundingBox, warnings, error)` — politique de
  clamping bord d'image : translation d'abord, réduction ensuite, échec en dernier recours.

**Port** — la seule interface du domaine, justifiée par la spec elle-même (§7/§12 signalent
explicitement Pigo comme un choix technique révisable après tests) :
- `FaceDetector interface { Detect(SourceImage) ([]DetectedFace, error) }` — l'implémentation infra
  (Pigo) doit toujours retourner des coordonnées dans le repère de l'image source pleine résolution,
  même si elle downscale en interne pour la performance (aucun downscaling prévu pour l'instant —
  YAGNI tant que les seuils de performance de §11 ne l'exigent pas).

**Infra (hors domaine)**, consommant `NormalizedPortrait`/`PortraitDiagnostic` :
- `JPEGPortraitExporter` (qualité 75 pour le prototype initial).
- `DiagnosticPDFExporter` (via `go-pdf/fpdf`) — dessine `SourceImage` + `FaceBox` (rouge) +
  `CropBox` (vert) en haut, `NormalizedPortrait` en dessous.
- Redimensionnement (crop → résolution de sortie) via `golang.org/x/image/draw` (`CatmullRom`).
- Lecture du tag EXIF `Orientation` (parsing minimal, sans dépendance externe) avant détection.
- Le cascade Pigo (`facefinder`) doit être embarqué via `go:embed` pour un binaire autonome (§11
  de la spec).

## Commandes

Aucun outillage de build/test/lint n'existe pour l'instant. Une fois les premiers fichiers source
ajoutés, le workflow Go standard s'applique : `go build ./...`, `go test ./...`, `go vet ./...`,
`go test ./chemin/vers/pkg -run TestName` pour un test unique.
