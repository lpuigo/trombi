# Spécification fonctionnelle — Générateur de trombinoscope

Statut : brouillon de cadrage — aucune implémentation démarrée.

## 1. Objectif

Fournir un outil qui, à partir d'un ensemble de fichiers image (photos d'identité individuelles, formats variés), détecte automatiquement le visage sur chaque photo et produit un portrait recadré selon une règle uniforme, en vue de constituer un trombinoscope d'équipe.

## 2. Périmètre

### Dans le périmètre
- Traitement d'un lot d'images (un dossier en entrée) contenant chacune un portrait individuel.
- Détection automatique du visage sur chaque image.
- Calcul et application d'un cadrage uniforme (ratio, marges, résolution) autour du visage détecté.
- Export des images recadrées dans un dossier de sortie, avec convention de nommage prévisible.
- Deux modes d'utilisation : ligne de commande et interface graphique.
- Fonctionnement entièrement local, sans transmission des images ou des données vers un service externe.

### Hors périmètre (au moins pour une première version)
- Reconnaissance/identification de personnes (associer un nom à un visage).
- Retouche d'image avancée (correction colorimétrique, suppression d'arrière-plan, etc.) — pourra être envisagée plus tard mais n'est pas un prérequis.
- Génération de la planche/mise en page finale du trombinoscope (l'outil produit les portraits recadrés, pas le document final).
- Traitement de photos de groupe (plusieurs personnes destinées à devenir plusieurs fiches) — le cas "plusieurs visages sur une image source" est traité comme cas limite (voir §8), pas comme fonctionnalité cible.

## 3. Contraintes générales

- **Traitement 100% local** : aucune image, ni aucune métadonnée, n'est envoyée à un service tiers ou une API cloud. Cette contrainte exclut d'office les solutions de détection basées sur des API cloud (AWS Rekognition, Google Vision, Azure Face, etc.).
- **Langage** : Go, en limitant au maximum les dépendances externes (librairies tierces, bindings CGo, dépendances système à installer séparément).
- **Portabilité de déploiement** : l'outil doit pouvoir être distribué et exécuté simplement sur Windows, macOS et Linux, avec un minimum de prérequis d'installation côté utilisateur final.
- **Environnement de développement** : le développement et la mise au point se font sous Windows ; le déploiement doit néanmoins être simple sur Mac (et Linux). Cette contrainte pousse à privilégier des approches qui compilent nativement en cross-compilation depuis Windows (Go pur, sans CGo) plutôt que des approches nécessitant une chaîne de compilation ou un SDK spécifique à la plateforme cible.
- **Simplicité d'intégration** : à choix technique équivalent, préférer la solution la plus simple à intégrer et à maintenir plutôt que la plus précise.

## 4. Utilisateurs cibles

- Utilisateur technique (ligne de commande), pour un traitement en masse ou scripté.
- Utilisateur non informaticien, via l'interface graphique, pour un usage ponctuel (ex. constitution annuelle du trombinoscope).

## 5. Entrées / Sorties

### Entrées
- Un dossier contenant les photos sources.
- Formats image supportés : JPEG et PNG au minimum (à confirmer/étendre selon besoin : BMP, TIFF, HEIC — ce dernier pose potentiellement un problème de dépendance et sera à évaluer séparément).
- Une photo = un portrait attendu (cas nominal : un seul visage par image).

### Sorties
- Un dossier contenant les images recadrées, une par photo source traitée avec succès.
- Convention de nommage à définir (par défaut : nom du fichier source conservé, écrit dans le dossier de sortie, format de sortie configurable).
- Un rapport de traitement (au minimum en sortie console/log) listant :
  - les fichiers traités avec succès,
  - les fichiers en échec (aucun visage détecté, plusieurs visages détectés, format non supporté, image illisible…) pour permettre une reprise manuelle.

## 6. Fonctionnement général (pipeline)

1. Lecture du dossier source, recensement des fichiers image valides.
2. Pour chaque image :
   a. Détection du/des visage(s).
   b. Application de la règle de sélection en cas d'ambiguïté (0 ou plusieurs visages détectés) — voir §8.
   c. Calcul du rectangle de cadrage à partir de la position/taille du visage et des paramètres de cadrage (§7).
   d. Recadrage, redimensionnement à la résolution de sortie, export.
3. Génération du rapport de traitement (succès / échecs / avertissements).

## 7. Détection de visage

- **Solution retenue pour une première version** : [Pigo](https://github.com/esimov/pigo), bibliothèque de détection de visages 100% Go, sans dépendance CGo ni bibliothèque système externe (pas de dépendance à OpenCV).
- Choix motivé par la contrainte de simplicité de déploiement/cross-compilation (§3) : une dépendance Go pure évite les complications de build multi-plateforme depuis un poste de développement Windows.
- Cette solution sera évaluée à l'usage sur un échantillon représentatif de photos réelles lors de la phase de test. Si la robustesse s'avère insuffisante (poses non frontales, éclairages difficiles, etc.), une alternative plus robuste (ex. détecteur DNN via OpenCV/gocv, au prix d'une dépendance CGo) sera reconsidérée — ce point est un **point ouvert**, à trancher après tests.
- Si la bibliothèque retenue permet également la localisation de points caractéristiques (ex. pupilles, chez Pigo via un module dédié de localisation de pupille), cette information pourra être utilisée pour améliorer le cadrage (redressement de l'image si la tête est inclinée) — fonctionnalité à considérer comme une amélioration possible plutôt qu'un prérequis de la première version.
- **Précisions confirmées sur l'API Pigo (`pigo/core`, v1.4.6)**, structurantes pour le cadrage (§9) et la gestion des détections multiples (§8) :
  - `Detection{Row, Col, Scale, Q}` n'a qu'un seul `Scale` : la bounding box native d'un visage détecté par Pigo est **toujours carrée** (même valeur pour largeur et hauteur).
  - `Q` est un score de classifieur en cascade, **non normalisé** (pas une probabilité entre 0 et 1) — un seuil de filtrage doit être calibré empiriquement sur un échantillon réel, pas fixé a priori (voir §12).
  - Pigo fournit `ClusterDetections(detections, iouThreshold)`, qui fusionne par IoU les détections qui se chevauchent (moyenne non pondérée de position/taille/score) — cette fusion doit être appliquée systématiquement avant toute règle métier de sélection (§8), pour éviter que le bruit naturel du détecteur (plusieurs détections sur un même visage réel) ne déclenche à tort un avertissement « visages multiples ».
  - Le cascade de classification (fichier `facefinder`) doit être embarqué dans le binaire via `go:embed` pour respecter la contrainte de binaire autonome sans dépendance externe (§11).
- **Orientation EXIF** : `image/jpeg` (stdlib) ignore le tag EXIF `Orientation` — une photo de téléphone taguée « à pivoter » serait décodée de travers, ferait échouer la détection de visage, et remonterait dans le rapport comme un « aucun visage détecté » sans cause visible. Décision retenue : lire le tag EXIF `Orientation` via un parsing minimal, sans dépendance externe, et réorienter l'image en mémoire avant détection ; en l'absence de tag ou en cas d'échec de parsing, l'image est traitée telle quelle.

## 8. Gestion des cas limites de détection

| Cas | Comportement attendu |
|---|---|
| Aucun visage détecté | Fichier marqué en échec, non traité, signalé dans le rapport pour revue manuelle. |
| Un seul visage détecté | Cas nominal, traitement standard. |
| Plusieurs visages détectés | **Comportement retenu** : 1) fusion des détections par IoU (`ClusterDetections`, voir §7) pour éliminer les doublons issus du bruit du détecteur ; 2) élimination optionnelle des détections dont le score `Q` est sous un seuil configurable (désactivé par défaut tant qu'il n'a pas été calibré sur un échantillon réel — voir §12) ; 3) sélection du visage de plus grande aire parmi les survivants. Avertissement systématique dans le rapport dès qu'il reste plus d'un visage après fusion, même si le fichier a été traité automatiquement. |
| Image illisible / format non supporté | Fichier ignoré, signalé en erreur dans le rapport. |
| Photo orientée selon un tag EXIF non pris en compte par le décodeur (ex. photo de téléphone pivotée) | **Comportement retenu** : lecture minimale du tag EXIF `Orientation` (sans dépendance externe) et réorientation de l'image en mémoire avant détection — voir §7. |
| Visage détecté trop proche du bord de l'image (cadrage calculé sortant du cadre source) | **Comportement retenu** : 1) translation de la box de crop (taille inchangée) pour la ramener dans les limites de l'image si géométriquement possible ; 2) sinon réduction de la box en conservant le ratio de sortie, avec avertissement dans le rapport ; 3) échec signalé uniquement si la bounding box native du visage dépasse elle-même les limites de l'image (cas pathologique). |

## 9. Cadrage du portrait

### 9.1 Principe
Le cadrage final est calculé à partir de la bounding box du visage détecté, à laquelle sont appliqués des marges et un ratio configurables, pour produire un rectangle de recadrage centré sur le visage.

### 9.2 Paramètres ajustables par l'utilisateur

**Décision de réconciliation (analyse DDD, 2026-09-03)** : les marges horizontale/haute/basse et le
ratio de sortie, pris indépendamment, sur-déterminent le rectangle de cadrage (4 nombres pour 3
degrés de liberté réels — largeur, hauteur, ratio = largeur/hauteur) ; cette sur-détermination est
amplifiée par le fait que la bounding box native retournée par Pigo est toujours carrée (§7), ce
qui rend le résultat des marges par défaut ci-dessous incompatible avec le ratio 3:4 si les deux
étaient appliquées indépendamment. Le couple **marge supérieure + marge inférieure** est retenu
comme source de vérité :

- La **hauteur** de la box de crop est déterminée par la marge supérieure et la marge inférieure
  (en % de la hauteur du visage détecté).
- La **largeur** de la box de crop est **dérivée** : `largeur = hauteur × ratio de sortie`.
- La **marge horizontale effective** et la **position verticale du visage dans le cadre** deviennent
  des valeurs **calculées**, affichées à titre indicatif (y compris dans la future interface
  graphique), et non plus des champs saisissables indépendamment.

Tous les paramètres marqués « source de vérité » doivent être modifiables par l'utilisateur (via
option en ligne de commande et/ou champ dans l'interface graphique et/ou fichier de configuration),
avec une valeur par défaut permettant un résultat correct dans le cas d'usage standard (photo de
face, cadrage buste/visage). Les valeurs par défaut ci-dessous sont des points de départ, à
affiner lors des phases de test finales — elles ne sont pas figées.

| Paramètre | Rôle | Valeur par défaut indicative |
|---|---|---|
| Marge supérieure | Source de vérité (hauteur de la box) | 30 % de la hauteur du visage (resserré le 2026-09-03 depuis 35 %, sur retour visuel via le PDF de diagnostic) |
| Marge inférieure | Source de vérité (hauteur de la box) | 47 % de la hauteur du visage (resserré le 2026-09-03 depuis 55 %, idem) |
| Ratio de sortie (largeur:hauteur) | Source de vérité (largeur = hauteur × ratio) | 3:4 |
| Résolution de sortie | Dimensions en pixels de l'image exportée | 600 × 800 px |
| Marge horizontale effective | **Dérivée** (affichage seul) | ≈ 16 % de chaque côté (calculée à partir d'une box native carrée — voir §7 ; recalculée à chaque image, non figée) |
| Position verticale du visage dans le cadre | **Dérivée** (affichage seul) | ≈ 45 % depuis le haut |
| Format de fichier de sortie | JPEG ou PNG | JPEG, qualité 90 (configurable) |
| Redressement automatique (rotation) | Activé si les données de landmarks sont disponibles et fiables | Désactivé par défaut en v1 (amélioration possible) |

**Point ouvert restant** : « Résolution de sortie » et « Ratio de sortie » sont aujourd'hui deux
champs indépendants qui peuvent diverger (ex. résolution 500×500 avec ratio 3:4) — voir §12.

### 9.3 Configuration
Ces paramètres doivent pouvoir être :
- fournis en ligne de commande (options), pour un usage scripté/reproductible,
- modifiés dans l'interface graphique, pour un ajustement interactif par un utilisateur non technique,
- éventuellement persistés dans un fichier de configuration (ex. YAML/JSON/TOML) rechargeable, pour éviter d'avoir à ressaisir les mêmes réglages à chaque exécution — point à confirmer selon le besoin réel.

### 9.4 Méthodes d'export du résultat d'une image

Deux méthodes d'export du traitement d'une image sont prévues, construites sur le même modèle de
données interne (image source, bounding box native du visage, bounding box de crop, portrait
recadré) afin de pouvoir être réutilisées sans adaptation par une future interface web (§10.2) :

- **Export JPEG** : le portrait recadré et redimensionné, seul, au format JPEG. Qualité retenue
  pour la première implémentation : **75** (la qualité 90 du tableau §9.2 reste la valeur par
  défaut du paramètre configurable à terme ; 75 est une valeur figée pour ce premier export, à
  aligner ultérieurement si besoin).
- **Export PDF de diagnostic** : un PDF d'une page contenant, en haut, l'image source complète
  avec en surimpression les deux bounding boxes (bounding box native du visage détecté, et
  bounding box de crop utilisée pour produire le résultat), dans deux couleurs distinctes
  (par défaut : rouge = bounding box native, vert = bounding box de crop), et en dessous le
  portrait recadré résultant.

**Dépendances retenues** (Go pur, cohérent avec §3) :
- Génération PDF : `go-pdf/fpdf` (fork maintenu de `gofpdf`, archivé).
- Redimensionnement d'image (crop → résolution de sortie) : `golang.org/x/image/draw` (filtre
  `CatmullRom`) — la bibliothèque standard `image` ne fournit pas de rééchantillonnage.

## 10. Interfaces utilisateur

### 10.1 Ligne de commande
- Exécution batch sur un dossier, avec les paramètres de cadrage en options.
- Doit permettre un traitement non interactif (scriptable), avec code de retour et rapport exploitable.

### 10.2 Interface graphique
- Objectif : rendre l'outil utilisable par une personne non informaticienne (sélection du dossier source/destination, ajustement des paramètres de cadrage, lancement du traitement, visualisation des résultats/erreurs).
- Contrainte : solution **portable** (Windows/Linux/Mac) et cohérente avec la contrainte de dépendances minimales et de simplicité de cross-compilation depuis un poste Windows (§3).
- Pistes à évaluer (à trancher avant développement, hors périmètre de ce document de cadrage) :
  - **Serveur HTTP local embarqué (Go standard library) + interface web (HTML/JS) ouverte dans le navigateur par défaut** : n'introduit aucune dépendance CGo ni toolkit graphique natif, cross-compilation triviale depuis Windows vers Mac/Linux, cohérent avec le choix déjà fait pour Pigo (Go pur). Piste à privilégier a priori compte tenu des contraintes exprimées.
  - Frameworks GUI natifs Go (ex. Fyne, Wails) : expérience utilisateur plus "application native", mais reposent en général sur CGo et/ou un toolkit graphique système (OpenGL, WebView natif), ce qui complique la cross-compilation depuis Windows vers une cible Mac et va à l'encontre de la contrainte de dépendances minimales. À écarter sauf besoin explicite justifiant le surcoût.
- Ce choix technique est à valider avant le début du développement.

## 11. Exigences non fonctionnelles

- **Confidentialité** : aucune donnée (image, chemin de fichier, métadonnées) ne doit transiter par un réseau externe ; l'outil doit pouvoir fonctionner sans connexion internet.
- **Déploiement** : livraison sous forme de binaire(s) autonome(s) par plateforme (Windows/macOS/Linux), sans installation de dépendances système lourdes côté utilisateur final (pas d'installation d'OpenCV, de runtime Python, etc.).
- **Performance** : traitement d'un lot de photos (ordre de grandeur : quelques dizaines à quelques centaines d'images) en un temps raisonnable sur un poste bureautique standard — seuil précis à définir si nécessaire.
- **Robustesse** : un échec de traitement sur une image ne doit pas interrompre le traitement du lot ; il doit être isolé et rapporté.
- **Traçabilité** : le rapport de traitement doit permettre à l'utilisateur d'identifier rapidement les photos nécessitant une intervention manuelle.

## 12. Points ouverts à trancher (avant ou pendant la phase de développement)

- Robustesse réelle de Pigo sur l'échantillon de photos de l'équipe → conditionnera le maintien ou non de ce choix.
- ~~Comportement par défaut exact en cas de détection multiple ou de cadrage débordant du cadre source (§8).~~ **Résolu** — voir §8.
- Choix technique définitif pour l'interface graphique (§10.2).
- Formats image supplémentaires à supporter (HEIC notamment, potentiellement problématique en dépendances).
- Persistance ou non des paramètres de cadrage dans un fichier de configuration.
- Convention de nommage précise des fichiers de sortie.
- Valeurs par défaut définitives des paramètres de cadrage (§9.2), à ajuster lors des tests finaux sur photos réelles.
- **Seuil de score minimal `Q`** (score de classifieur Pigo non normalisé) pour écarter les faux
  positifs de détection avant d'appliquer la règle « visage de plus grande aire » (§8) — ne peut
  être fixé qu'empiriquement, lors des tests sur photos réelles (§13.2) ; désactivé par défaut en
  attendant.
- **Cohérence « Résolution de sortie » / « Ratio de sortie »** (§9.2) : ces deux champs sont
  aujourd'hui indépendants et peuvent diverger — à trancher : dériver l'un de l'autre (ex. la
  résolution définit le ratio) ou valider strictement leur cohérence à la saisie.

## 13. Étapes suivantes suggérées

1. Validation de cette spécification.
2. Constitution d'un petit échantillon de photos représentatif (variété de cadrages sources, éclairages, orientations) pour tester Pigo avant tout développement d'outil complet.
3. Décision sur la piste GUI (§10.2).
4. Développement d'un prototype CLI minimal (détection + cadrage avec valeurs par défaut) pour valider la chaîne de traitement de bout en bout.
5. Ajout de l'interface graphique une fois le cœur de traitement validé.
