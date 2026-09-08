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

| Rôle ou droit                          | Effet                                                                        |
|:---------------------------------------|:-----------------------------------------------------------------------------|
| Anonyme                                | Lit `PUBLIC` et les chemins exacts connus de `HIDDEN`                        |
| `base`                                 | Compte authentifié sans écriture implicite                                   |
| `canview:{repo}` / `canview:*`         | Lit le dépôt nommé ou tous, y compris privés                                 |
| `canmoderate:{repo}` / `canmoderate:*` | Examine et décide le contenu en attente du dépôt nommé ou de tous les dépôts |
| `canupdate:{repo}` / `canupdate:*`     | Publie dans le dépôt, sous réserve de la politique paquet/domaine            |
| `showing`                              | Droit historique permettant de découvrir les dépôts cachés dans le catalogue |
| `allview` / `proview`                  | Alias historiques de lecture privée globale                                  |
| `manager` / `admin`                    | Super-administrateur système et de toutes les équipes                        |

L’administration système est globale. Les niveaux L0-L4 restent l’autorité normale de collaboration. Une opération
administrateur est auditée et n’ajoute pas silencieusement un membre affiché.
La modération inclut la visibilité privée nécessaire à l’examen, sans autoriser publication, gestion des utilisateurs,
configuration des dépôts ni paramètres système.

Les administrateurs et modérateurs ne peuvent pas être suspendus tant qu’ils conservent ces rôles. Retirez toutes leurs permissions d’administration et de modération avant de suspendre leur compte. Un compte suspendu doit être rétabli avant de recevoir ces rôles. La base vérifie les deux opérations dans la transaction du compte, y compris les changements de permissions et de nom. Les conflits renvoient `409` avec `ACCOUNT_BAN_PROTECTED` ; une suspension refusée préserve les sessions existantes.

## Suspension du compte et des IP

Les administrateurs système peuvent suspendre un compte depuis la page des utilisateurs, avec un motif, une expiration facultative et l’option **Bloquer aussi les IP de connexion enregistrées**. Le serveur collecte au maximum 64 adresses normalisées à partir des sessions conservées (les 64 dernières actives) et des 256 dernières connexions réussies sur 30 jours. Il conserve aussi les adresses déjà couvertes par une suspension IP active. Le navigateur ne peut pas fournir d’adresses arbitraires. Sans adresse utilisable, toute l’opération échoue avec `409 ACCOUNT_BAN_IP_UNKNOWN` ; désactivez l’option pour suspendre uniquement le compte.

Les restrictions IP, la suspension et la révocation des sessions sont validées ensemble. Les restrictions survivent aux redémarrages et changements de nom, et expirent avec la suspension. Le démarrage conserve les identités définitivement fermées sans recréer leurs identifiants ni leurs appartenances. Décocher l’option retire les restrictions IP de ce compte en conservant sa suspension ; rétablir le compte retire les deux. Une adresse reste bloquée si la suspension active d’un autre compte la couvre encore. Omettre `ban_ip` lors d’une modification conserve les restrictions IP actives.

Une adresse bloquée reçoit `403` avec `IP_BANNED` pour ses requêtes HTTP, dont la connexion, l’inscription, les requêtes authentifiées et les téléchargements publics. Les autres utilisateurs partageant l’adresse sont aussi affectés. Les adresses transmises ne sont acceptées que depuis les proxys de confiance configurés. Le statut administrateur expose un nombre, pas les adresses enregistrées ; tous ces endpoints nécessitent les droits d’administrateur système et renvoient des métadonnées privées non mises en cache. Les jetons API doivent aussi inclure la portée `admin:users`.

| Méthode | Chemin | Requête ou résultat |
|---|---|---|
| GET | `/api/tokens/:name/ban` | `{ban,ip_count,protected_role}` |
| PUT | `/api/tokens/:name/ban` | `{reason,expires_at,ban_ip}` ; expiration éventuellement null |
| DELETE | `/api/tokens/:name/ban` | Lever la suspension et les restrictions IP du compte ; `204` |

## Couches dépôt et équipe

- La visibilité définit découverte et lecture de base : `PUBLIC`, découverte de `HIDDEN` selon les droits ou `PRIVATE`
  autorisé.
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

Un `403` pendant la restauration de session ne déconnecte pas le navigateur : une restriction IP ou d’autorisation peut concerner une session encore valide. Un `401` nécessite une nouvelle connexion.

## Défense en profondeur

- Mots de passe et codes utilisent une vérification salée irréversible ; le secret API Token n’est pas persisté.
- Les sessions expirent, sont révocables par appareil, et une récupération les révoque toutes atomiquement.
- Limites, bannissements progressifs, plafond actif et proxys fiables protègent le réseau.
- Uploads, archives, miroirs et mises à jour emploient streaming borné, validation de chemin, hashes et stockage
  temporaire.
- Audit et messages conservent les résultats pertinents sans révéler l’opérateur lorsque la notification doit être
  neutre.
