---
title: Registre Docker et OCI
order: 3
category: Guides
description: Créer des images et utiliser Docker, Podman, containerd ou nerdctl avec RenoP
---

# Guide du registre Docker et OCI

Créez un dépôt de format `docker`, puis chaque image cible avant le push. Les exemples utilisent le dépôt `containers`
et l’image `team/service`, soit le nom de registre `containers/team/service`.

## Connexion et transport

```bash
docker login localhost:3000
# Username: admin
# Password: <your_password_or_API_token>
```

Utilisez un API Token dédié : `repository:read` pour pull, `repository:publish` pour push, `repository:delete` pour la
suppression, `package:create` pour réserver une image via l’API et `team:manage` pour les collaborateurs. Le jeton Docker
court ne reçoit que les actions permises par scopes/cibles et par la politique L0-L4 actuelle.

Utilisez HTTPS en production. Pour un test HTTP local uniquement :

```json
{
  "insecure-registries": ["localhost:3000"]
}
```

Redémarrez Docker après `daemon.json`. Podman et containerd disposent de paramètres de confiance équivalents.

## Créer, taguer et pousser

Ouvrez `containers`, créez `team/service` et choisissez public ou privé. Une image privée n’accorde aucun L0 implicite ;
ajoutez lecteurs et collaborateurs dans son équipe. Les composants du nom sont en minuscules.

La création échoue si le nom existe localement ou sur un miroir applicable. Une vérification amont indéterminée ne
réserve pas le nom. Les images découvertes par miroir restent en lecture seule.

```bash
# Tag local image
docker tag service:latest localhost:3000/containers/team/service:1.0.0

# Push image to RenoP
docker push localhost:3000/containers/team/service:1.0.0
```

RenoP refuse le droit push, le début d’upload et le manifeste avant la création de l’image. Après un échec, les retries
restent valides sans rouvrir le login ou la fenêtre du navigateur.

Avec l’une ou l’autre politique, la création de l’image renvoie `202 Accepted` et ne réserve pas le nom avant
l’approbation. Sous `new_packages`, les push suivants s’exécutent normalement. Sous `every_version`, chaque envoi de
manifeste renvoie aussi un identifiant d’examen et reste absent des réponses pull, tag-list et catalogue. L’approbation
revérifie l’éditeur et les blobs avant de publier atomiquement le tag. Le rejet ne touche que le manifeste virtuel ; les
blobs partagés et tags existants restent intacts. Les imports de miroir ne sont pas examinés.

## Pull et exécution

```bash
# Pull image
docker pull localhost:3000/containers/team/service:1.0.0

# Run container
docker run -d -p 8080:8080 localhost:3000/containers/team/service:1.0.0
```

Une image publique est anonyme. Une image privée exige un membre L0-L4 ou administrateur. L’accès blob reste lié à
l’image : connaître un digest d’une autre image ne donne aucun droit.

## Comportement OCI

- **Multi-architecture** : listes et index OCI pour amd64, arm64 et autres plateformes.
- **Uploads découpés** : flux POST/PATCH/PUT repris avec stockage temporaire borné.
- **Mount inter-dépôts** : lecture de la source et écriture sur une destination précréée.
- **Suppression** : capacité du Token et autorisation image/dépôt sont toutes deux requises.
- **Miroirs** : réponses diffusées et cataloguées avec leur origine ; une image miroir ne peut pas être poussée.
