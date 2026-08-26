---
title: Token et signatures GPG
order: 2
category: Sécurité
description: Identifiants fins, récupération et vérification OpenPGP des publications
---

# Token et signatures GPG

RenoP sépare sessions navigateur, API Token, mots de passe, récupération et clés de signature. Leur stockage, transport
et révocation diffèrent.

## API Token et récupération

Un API Token contient 256 bits aléatoires et le préfixe `rnp_pat_`. Le secret est affiché une fois ; seul son digest
SHA-256 est stocké. Chaque Token possède nom privé, scopes, cibles exactes facultatives et expiration facultative. Un
compte possède au plus 50 Token et un Token 128 cibles.

Appliquez privilège minimal et durée courte. Le Token et les droits actuels du compte doivent tous deux autoriser la
cible. La révocation efface immédiatement les caches. Les anciens secrets en clair migrent vers des identifiants hashés.

La session reste dans le cookie. Basic est limité aux protocoles de paquets. L’automatisation envoie
`Authorization: Bearer <token>`. Les identifiants en query sont ignorés ou refusés.

Les codes de récupération sont distincts. Un ensemble contient douze codes à usage unique, stockés comme vérificateurs
Argon2id. Quatre codes distincts réinitialisent atomiquement le mot de passe, sont consommés, révoquent les sessions et
réactivent le mot de passe. Conservez-les hors ligne et remplacez l’ensemble après usage ou fuite supposée.

---

## Vérification OpenPGP détachée

Un dépôt Maven peut exiger une signature `.asc` valide avant exposition. L’utilisateur enregistre sa clé publique ; la
clé privée n’entre jamais dans RenoP.

### Activer la vérification

```yaml
repositories:
  releases:
    name: releases
    format: maven
    require_gpg_signature: true
```

### Flux de publication

1. RenoP diffuse l’artefact dans `.renop.tmp.gpg` et crée une publication en attente bornée.
2. Le `.asc` correspondant peut arriver avant ou après l’artefact dans le délai.
3. RenoP résout un fingerprint enregistré non ambigu, vérifie signature, éditeur et politique sous le verrou du dépôt.
4. La paire valide est commitée atomiquement et ses métadonnées sont enregistrées pour l’interface.
5. Toute publication invalide, absente, expirée, supprimée ou non autorisée échoue avec une raison stable.

Les serveurs de clés utilisent HTTPS sous `server.gpg.key_servers`. Les requêtes suivent la politique proxy, utilisent
des clients bornés et n’envoient jamais de clé privée.
