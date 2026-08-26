---
title: Jetons API et GPG
order: 2
category: Sécurité
description: Jetons API à permissions fines et signatures GPG
---

# Jetons API et GPG

- **Jetons API** : Identifiants nommés de 256 bits, avec permissions et expiration facultative. Le secret n'est affiché
  qu'une fois et seule son empreinte SHA-256 est conservée.
- **Autorisation** : Chaque requête est limitée à la fois par les permissions du jeton et par les droits actuels du
  compte. La révocation prend effet immédiatement.
- **Transport** : Utilisez `Authorization: Bearer <token>` pour les API. Basic est réservé aux protocoles de paquets ;
  les sessions dans les en-têtes et les identifiants dans l'URL sont refusés.
- **GPG** : Vérification des signatures `.asc` et mise en quarantaine dans `.renop.tmp.gpg`.
