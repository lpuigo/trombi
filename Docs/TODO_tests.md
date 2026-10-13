# Suivi des tests manuels (GUI web)

Remarques et anomalies identifiées lors des sessions de test manuel de la GUI web, à trier/prioriser
avant implémentation. Non exhaustif, alimenté au fil des sessions de test.

## Ouvert

_Aucun point ouvert pour le moment._

## Fait

- [x] **Boîtes visage/cadrage déformées (format paysage) sur les photos portrait** : sur IMG_7511
  (photo pivotée via EXIF orientation=6), les rectangles rouge (visage) et vert (cadrage) de
  l'éditeur de cadrage s'affichaient plus larges que hauts, alors que le visage et le cadrage sont
  bien portrait. Diagnostic confirmé par test direct du backend (hors UI) : `SourceImage 240x320`,
  `FaceBox 70x70` (carré, normal), `CropBox 92x124` (portrait, ratio ≈3:4) — le calcul serveur est
  donc correct, le bug était uniquement côté rendu. Cause : `#editor-boxes` avait
  `preserveAspectRatio="none"` (`webserver/static/index.html`) ; combiné à `.source img { width:100%;
  max-height:60vh; object-fit:contain }` (`webserver/static/style.css:260-261`), la boîte CSS de
  l'`<img>` peut avoir un ratio différent de la photo réelle (letterboxée par `object-fit:contain`),
  et le SVG s'étirait pour remplir cette boîte sans respecter son propre `viewBox`. Corrigé en
  retirant `preserveAspectRatio="none"` (valeur par défaut `xMidYMid meet`, qui letterboxe le SVG
  exactement comme `object-fit:contain` letterboxe l'image). Vérifié visuellement sur IMG_7511 +
  `go test ./...` (aucune régression).

- [x] **Pas de retour visuel pendant la génération du PDF** : le bouton "⬇ PDF" était un simple
  `<a href="api/export.pdf" download>` (`webserver/static/index.html`) — un clic déclenchait un
  téléchargement natif du navigateur sans aucun retour visuel pendant que le serveur générait le
  PDF (`ExportPDF` peut prendre plusieurs secondes sur un gros lot), et rien n'empêchait de
  re-cliquer entre-temps. Corrigé : le clic est maintenant intercepté en JS
  (`webserver/static/app.js`, fin de la section Wiring) — le bouton est désactivé
  (classe `.disabled`, déjà utilisée ailleurs, bloque aussi les clics via `pointer-events:none`),
  une modale `#export-progress` (spinner + message) s'affiche pendant l'appel `fetch`, puis le PDF
  reçu est déclenché en téléchargement via un `Blob`/`URL.createObjectURL` et un `<a>` synthétique.
  Vérifié : modale + désactivation du bouton pendant l'appel, téléchargement effectif à la fin,
  aucune erreur console, `go test ./...` toujours vert. Ajustement suite retour : la modale
  centrée pleine largeur était trop imposante — repositionnée en haut à droite, près du bouton
  d'export (`#export-progress { width: min(40vw, 320px); margin: 4rem 1rem auto auto; }` dans
  `webserver/static/style.css`), vérifié visuellement avec un lot de 32 photos (génération assez
  longue pour observer la modale affichée).

- [x] **Délai d'affichage du tooltip d'avertissement trop long** : le triangle ⚠/✓/⏳ d'une photo
  (`renderIconBar()`, `webserver/static/app.js`) utilisait l'attribut HTML natif `title`, dont le
  délai d'apparition est géré par le navigateur et non configurable. Corrigé : remplacé par un
  tooltip custom en CSS (`status.dataset.tooltip` + `.status[data-tooltip]::after` dans
  `webserver/static/style.css`, `transition-delay: .15s`), avec support clavier (`tabIndex = 0` +
  `:focus`). Vérifié visuellement (apparence du tooltip custom confirmée, distincte du style natif
  du navigateur) + `go test ./...` (changement purement frontend, aucune régression Go).

- [x] **Compte rendu d'import manquant** : lors de l'import d'un dossier contenant des fichiers
  rejetés (ex. `test/` → 34 fichiers importés, 2 PDF rejetés), seul un toast transitoire (6s, sans
  détail par fichier) signalait le rejet (`enqueueUploads()`, `webserver/static/app.js`). Corrigé en
  étendant la section persistante `#failures` déjà utilisée pour les échecs serveur (aucun visage
  détecté, format illisible) : un fichier rejeté côté client (extension non supportée) devient
  maintenant un item `local:true, status:"failure"` avec son nom et sa raison, listé dans
  `#failures` au lieu d'un toast — cohérent avec l'affichage déjà en place, pas de nouveau
  composant. Correctif associé : `removeItem()` ne doit pas appeler `api.remove()` pour un item
  `local` (n'existe pas côté serveur) — sinon le bouton corbeille de la liste échouait avec une
  erreur 404. Vérifié visuellement (import de 2 PDF + 2 JPEG, les 2 PDF apparaissent dans
  "Photos non utilisables" avec leur raison, suppression sans appel serveur ni erreur console).

- [x] **Décalage horizontal manuel du cadrage (3ᵉ slider)** : besoin exprimé — pouvoir décaler
  volontairement le cadrage par rapport au centre du visage détecté (ex. pour compenser une
  détection imprécise). Décision prise avec l'utilisateur : réviser `Docs/SPEC_trombinoscope.md`
  §9.2 pour ajouter un vrai degré de liberté horizontal — ce n'est pas une sur-détermination
  (contrairement à l'ancienne idée de marge horizontale symétrique indépendante) car un décalage ne
  fait que **translater** la box de crop déjà dimensionnée par marges/ratio, sans toucher à ses
  3 degrés de liberté réels (largeur, hauteur, ratio).
  Implémenté sur toutes les couches :
  - **Domain** (`domain/portrait/framing.go`) : nouveau champ `FramingSpec.HorizontalOffset`
    (fraction de la largeur du visage, signé) ; `WithMargins(top, bottom)` renommé
    `WithFraming(top, bottom, horizontalOffset)` ; `ComputeCropBox` translate la box de
    `offset × largeur_visage`. `HorizontalMargin()` reste la marge symétrique de référence
    (offset = 0), devient asymétrique en pratique si offset ≠ 0 (commentaire mis à jour). Nouveau
    test `TestComputeCropBoxHorizontalOffset`.
  - **Service** (`service/workspace.go`) : `SetItemMargins` renommé `SetItemFraming(id, top,
    bottom, horizontalOffset)`.
  - **Webserver** (`webserver/items.go`, `webserver/dto.go`) : `itemPatch.HorizontalOffsetPct`,
    `framingDTO`/`itemDTO.HorizontalOffsetPct` ; les 3 champs (marges + décalage) doivent être
    fournis ensemble dans un PATCH, comme avant pour les 2 marges.
  - **Frontend** (`webserver/static/{index.html,app.js,style.css}`) : 3ᵉ slider "Décalage
    horizontal" dans l'éditeur par photo (-50 % à +50 %, affiché avec signe explicite) et dans les
    réglages globaux par défaut ; `sendEditorMargins()` renommé `sendEditorFraming()`.
  - **Specs** : `Docs/SPEC_trombinoscope.md` §9.2 (nouvelle section "Révision 2026-10-13") et §10.2
    mis à jour ; `CLAUDE.md` (modèle de domaine `FramingSpec`) mis à jour.
  Vérifié : `go build ./...`, `go vet ./...`, `go test ./...` tous verts ; test visuel navigateur
  (slider à +27 % déplace le cadrage vert vers la droite sans bouger le visage rouge, résultat
  recentré en conséquence, champ également présent dans les réglages globaux).
