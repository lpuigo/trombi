# Règles de design — projets créés from scratch

Ce fichier sert de référence à charger (copier/coller ou `@` référencer) dans le `CLAUDE.md`
d'un nouveau projet pour orienter la structuration du code dès le départ. Les deux priorités
absolues sont : **(1) Single Responsibility** à tous les niveaux de granularité, et
**(2) une architecture en couches strictement respectée**, garantissant un code découplé,
navigable et testable indépendamment couche par couche.

Ne pas traiter ces règles comme une checklist bureaucratique à cocher après coup : elles doivent
guider la structure du projet **avant** d'écrire la première ligne de code métier (arborescence
de dossiers, découpage en packages/modules, sens des dépendances).

---

## 1. Single Responsibility — la règle n°1

Une seule raison de changer, à chaque niveau :

- **Fichier / classe / struct** : un fichier ne doit porter qu'un seul concept. Si un fichier
  mélange logique métier et accès disque/réseau, ou validation et persistance, il doit être scindé.
- **Fonction** : une fonction fait une chose et la nomme correctement. Si le nom contient "et"
  ("ValidateAndSave", "FetchAndTransform"), c'est un signal qu'il faut la découper.
- **Package / module** : un package = un domaine ou une préoccupation technique (persistance,
  routing HTTP, logging), jamais un fourre-tout (`utils`, `common`, `helpers` sont des signaux
  d'alerte — s'ils grossissent, c'est qu'ils cachent plusieurs responsabilités non nommées).
- **Handler / endpoint** : un handler orchestre (parse la requête, appelle la couche métier,
  sérialise la réponse) — il n'implémente jamais la logique métier lui-même.

**Test rapide** : si on ne peut pas décrire la responsabilité d'une unité de code en une phrase
sans "et"/"ou", elle en fait trop.

---

## 2. Architecture en couches — obligatoire

Toujours structurer le backend (et, adapté, le frontend) en couches explicites, avec un
**sens de dépendance unique et descendant** :

```
route / controller / handler   (HTTP, CLI, gRPC — jamais de logique métier ici)
        ↓
manager / service               (orchestration, use-cases, transactions)
        ↓
domain / model                  (règles métier, invariants, structs métier)
        ↓
persist / repository / infra    (accès disque, DB, API externes)
```

Règles strictes associées :

- **Une couche ne connaît que la couche immédiatement inférieure.** Le handler HTTP ne doit
  jamais importer le package de persistance directement — il passe toujours par la couche
  d'orchestration.
- **Aucune dépendance remontante.** La couche persistance ne doit jamais importer la couche
  métier ou HTTP. Si un import remonte, c'est une inversion de dépendance manquante à corriger
  (interface à extraire côté consommateur, pas côté fournisseur).
- **Chaque couche a une carte mentale claire** : un nouveau contributeur (humain ou agent) doit
  pouvoir dire "je cherche une règle de validation → couche domain", "je cherche comment une
  donnée est écrite sur disque → couche persist", sans ambiguïté.
- **Pas de couche "God" qui court-circuite les autres.** Un objet d'agrégation (type `Manager`)
  peut exposer une façade unique vers les couches inférieures pour simplifier les dépendances des
  handlers, mais il ne doit pas lui-même contenir de logique métier — il orchestre, il ne décide pas.

---

## 3. Organisation des packages/modules

- **Un dossier/package par domaine métier** (bounded context), pas par type technique. Préférer
  `model/invoice/`, `model/customer/` à un unique `models/` fourre-tout si le projet grossit.
- **Symétrie de structure** : si chaque domaine a un repository, un service et un modèle, tous
  les domaines doivent suivre le même schéma de nommage et d'organisation — la prévisibilité
  réduit la charge cognitive.
- **Pas de dépendances circulaires entre domaines.** Si deux domaines doivent s'échanger des
  données, extraire une interface commune ou un package tiers neutre plutôt que de les faire
  s'importer mutuellement.
- **Un fichier par entité/concept principal**, pas un fichier de 2000 lignes qui regroupe tout
  un domaine.

---

## 4. Découplage — dépendre d'abstractions, pas d'implémentations

- **Programmer contre des interfaces**, surtout aux frontières de couches (persistance,
  services externes, notifications). La couche métier définit l'interface dont elle a besoin ;
  la couche infra l'implémente (Dependency Inversion).
- **Interfaces minimales** (Interface Segregation) : une interface ne doit exposer que ce dont
  l'appelant a réellement besoin, pas l'intégralité des méthodes d'une implémentation concrète.
- **Composition plutôt qu'héritage.** Construire les comportements par embedding/composition de
  petites briques réutilisables plutôt que par hiérarchies de classes profondes.
- **Injection explicite des dépendances** (constructeur/factory), jamais de singletons globaux
  cachés ni d'état partagé implicite entre couches.
- **Cross-cutting concerns (auth, logging, retry) en middleware/décorateurs**, jamais dupliqués
  dans chaque handler.

---

## 5. Autres principes à respecter systématiquement

- **DRY, mais pragmatique** : factoriser une vraie duplication de logique, ne pas sur-factoriser
  une simple ressemblance accidentelle (trois lignes similaires valent mieux qu'une abstraction
  prématurée et rigide).
- **KISS** : préférer la solution la plus simple qui satisfait le besoin actuel réel.
- **YAGNI** : ne pas construire de flexibilité, de config ou d'abstraction pour un besoin
  hypothétique futur non exprimé.
- **Fail fast / erreurs explicites** : valider aux frontières (entrée utilisateur, API externe),
  faire confiance au code interne une fois validé. Ne pas ajouter de garde-fous défensifs pour
  des cas qui ne peuvent pas se produire dans le flux interne.
- **Configuration externalisée** (fichier de config, variables d'environnement), jamais de
  valeurs d'environnement en dur dans le code métier.
- **Logging structuré et centralisé**, pas de `fmt.Println`/`console.log` épars — un point
  d'entrée de logging par couche transversale.
- **Nommage qui documente l'intention** : un bon nom de fonction/type rend le commentaire
  inutile. Ne commenter que le POURQUOI non évident (contrainte cachée, contournement, piège),
  jamais le QUOI.

---

## 6. Anti-patterns à bannir explicitement

- **God object / God package** : une struct ou un package qui connaît et fait tout.
- **Fuite de couche** : logique SQL/fichier dans un handler HTTP, logique métier dans un
  repository, appel HTTP direct depuis la couche domain.
- **Duplication de repository/pattern** : ne pas maintenir deux mécanismes concurrents pour la
  même responsabilité (ex. un composant générique non fini à côté d'implémentations manuelles
  redondantes par domaine) — factoriser complètement ou ne pas commencer la factorisation.
- **Fourre-tout `utils`/`common`/`helpers`** grossissant sans limite — signe de responsabilités
  non identifiées qui méritent leur propre package nommé.
- **Handlers/fonctions à branches multiples non liées** (une fonction qui fait A si condition X,
  B si condition Y, sans lien logique entre A et B) — à scinder en fonctions dédiées.
- **Dépendances remontantes ou circulaires** entre couches ou entre domaines.

---

## 7. Checklist de démarrage d'un nouveau projet

Avant d'écrire la première feature, poser explicitement :

1. Quelles sont les couches du projet (route → service → domain → infra, ou équivalent) ?
2. Quel est le découpage en domaines métier (packages/modules), et sont-ils symétriques en
   structure ?
3. Quelles interfaces séparent les couches (notamment domain ↔ infra) ?
4. Où vit la configuration, et est-elle bien externalisée dès le départ ?
5. Où vit le logging transversal (auth, erreurs, requêtes), pour éviter la duplication ?
6. Est-ce que chaque nouveau fichier/package qu'on s'apprête à créer peut se décrire en une
   phrase sans "et" ?

Si une réponse est floue, clarifier la structure avant d'ajouter du code — pas après.
