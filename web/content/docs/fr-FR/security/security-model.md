---
title: Sécurité et autorisations
order: 1
category: Sécurité
description: Identifiants, droits de dépôt, équipes de paquets et défense en profondeur
---

# Sécurité et autorisations

RenoP autorise selon le type d’identifiant, la capacité du Token, le rôle du compte, la visibilité et l’équipe cible.
Aucun identifiant ne conserve un droit perdu par son compte.

## Rôles du compte et du système

| Rôle ou droit | Effet |
|:--------------|:------|
| Anonyme | Lit `PUBLIC` et les chemins exacts connus de `HIDDEN` |
| `base` | Compte authentifié sans écriture implicite |
| `canview:{repo}` / `canview:*` | Lit le dépôt nommé ou tous, y compris privés |
| `canupdate:{repo}` / `canupdate:*` | Publie dans le dépôt, sous réserve de la politique paquet/domaine |
| `showing` | Droit historique permettant de découvrir les dépôts cachés dans le catalogue |
| `allview` / `proview` | Alias historiques de lecture privée globale |
| `manager` / `admin` | Super-administrateur système et de toutes les équipes |

L’administration système est globale. Les niveaux L0-L4 restent l’autorité normale de collaboration. Une opération
administrateur est auditée et n’ajoute pas silencieusement un membre affiché.

## Couches dépôt et équipe

- La visibilité définit découverte et lecture de base : `PUBLIC`, découverte de `HIDDEN` selon les droits ou `PRIVATE` autorisé.
- Un droit de dépôt ne crée pas automatiquement un paquet npm/Cargo/Docker et ne vérifie pas un domaine Maven.
- Les équipes npm/Cargo/Docker utilisent L0 lecture, L1 publication, L2 cycle/métadonnées, L3 membres, L4 propriété.
- Une équipe Maven appartient à un domaine global vérifié et vaut dans tous les dépôts Maven.
- Une image Docker privée n’accorde aucun L0 public implicite ; les blobs restent liés aux images lisibles.
- Un paquet npm privé doit être scoped et exige un membre explicite ou un administrateur.

## Transports d’identifiants

- **Session navigateur** : cookie HttpOnly `renop_session`, exigé pour la sécurité privée et la gestion des Token.
- **Basic** : nom plus mot de passe ou API Token, uniquement pour les protocoles de paquets.
- **Bearer API Token** : capacité et cible exacte pour l’automatisation.
- **Docker Bearer** : jeton court limité par l’identifiant source et l’image.

`Authorization: Session`, secrets de session dans les URL et paramètres d’identifiants sont refusés. Scopes et cibles
sont toujours croisés avec les droits actuels.

## Défense en profondeur

- Mots de passe et codes utilisent une vérification salée irréversible ; le secret API Token n’est pas persisté.
- Les sessions expirent, sont révocables par appareil, et une récupération les révoque toutes atomiquement.
- Limites, bannissements progressifs, plafond actif et proxys fiables protègent le réseau.
- Uploads, archives, miroirs et mises à jour emploient streaming borné, validation de chemin, hashes et stockage temporaire.
- Audit et messages conservent les résultats pertinents sans révéler l’opérateur lorsque la notification doit être neutre.
