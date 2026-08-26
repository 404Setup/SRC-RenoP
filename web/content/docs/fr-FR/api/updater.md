---
title: API de mise à jour intégrée
order: 13
category: Référence API
description: Vérification, téléchargement, installation hors ligne et redémarrage
---

# API de mise à jour intégrée

Les mutations exigent une session administrateur ou un jeton API avec `admin:updates`. Les erreurs sont renvoyées en
JSON avec `X-Renop-Error-Code`, ce qui permet de les traduire sans afficher de chemin interne ni d’erreur réseau brute.

## Lire l’état

`GET /api/updater/status` renvoie le protobuf `UpdateState`. L’état vaut `idle`, `checking`, `available`, `downloading`,
`ready_to_restart` ou `error`. Pendant une installation en ligne, interrogez cette route pour suivre la progression.

## Vérifier le canal configuré

`POST /api/updater/check?channel=release|nightly` renvoie un `CheckResult` JSON. Le paramètre facultatif ne remplace le
canal que pour cette requête. Le résultat comprend la cible, le SHA-256, la taille, les notes et la plage de changements
encore conservée entre la version active et la cible.

## Démarrer une installation en ligne

`POST /api/updater/install` lance en arrière-plan le téléchargement borné, la vérification du condensat, l’extraction
Brotli/ZIP et la validation du binaire. La réponse est `{"status":"started"}` et ne redémarre pas le processus.

La progression de téléchargement est un toast temporaire. Les résultats durables et les échecs restent enregistrés
dans le centre de messages des administrateurs.

## Installer un paquet hors ligne

`POST /api/updater/upload` accepte le champ multipart `file` ou `package` avec un paquet `.br` brut ou un ancien `.zip`.
Pour un gros fichier, utilisez l’envoi découpé avec `purpose=updater`, puis
`POST /api/upload/chunked/{upload_id}/complete`.

Le serveur traite le paquet en flux dans un espace temporaire borné, valide la plateforme du binaire et renvoie
`ready_to_restart`. Les erreurs ne divulguent pas les chemins internes.

## Redémarrer

`POST /api/updater/restart` applique le binaire préparé, s’il existe, puis redémarre RenoP. La connexion peut se fermer
avant la réponse `{"status":"restarting"}`. L’interface officielle affiche un toast avant le redémarrage.

## Codes d’erreur stables

`X-Renop-Error-Code` peut contenir `forbidden`, `insufficient_space`, `missing_file`, `install_busy`, `invalid_package`,
`incompatible_binary`, `package_too_large`, `package_processing_failed`, `check_failed`, `notification_failed` ou
`restart_failed`.
