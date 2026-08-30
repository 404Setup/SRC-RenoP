---
title: Registre npm
order: 4
category: Guides
description: Réserver des paquets et utiliser npm, pnpm, Yarn ou Bun avec RenoP
---

# Guide du registre npm

Créez un dépôt de format `npm`, puis réservez chaque paquet depuis sa page avant toute publication. RenoP n'autorise
pas un client à créer implicitement un nom. Les exemples utilisent le dépôt `javascript` et le paquet
`@example/library`.

## Configurer un client

Créez un API Token expirant avec les droits de lecture et de publication du dépôt. N'ajoutez les droits de cycle de
vie ou de gestion d'équipe que si l'automatisation en a besoin. Pour un registre dédié, placez ceci dans `.npmrc` :

```ini
registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

Pour ne router qu'un scope vers RenoP, conservez le registre par défaut et configurez ce scope séparément :

```ini
@example:registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

Utilisez HTTPS hors d'un réseau local de développement fiable. Préférez les API Tokens pour l'automatisation ; les
mots de passe de compte restent limités à l'authentification des protocoles de paquets standard.

## Préparer et publier un paquet

Le nom réservé et le champ `name` de `package.json` doivent être identiques. Les versions suivent SemVer et deviennent
immuables après une publication réussie.

```json
{
  "name": "@example/library",
  "version": "1.0.0",
  "description": "Example library",
  "publishConfig": {
    "registry": "https://packages.example.com/javascript/"
  }
}
```

Publiez et installez le paquet avec un client compatible :

```bash
npm publish
npm install @example/library
pnpm add @example/library
yarn add @example/library
bun add @example/library
```

RenoP valide l'archive gzip bornée, vérifie que `package/package.json` correspond à la requête, calcule les valeurs
d'intégrité SHA-1 et SHA-512 compatibles npm, puis valide le stockage uniquement après tous les contrôles.

Avec l’une ou l’autre politique, la création du paquet renvoie `202 Accepted` et ne réserve pas le nom avant
l’approbation. Sous `new_packages`, les commandes `npm publish` suivantes s’exécutent normalement. Sous
`every_version`, chaque publication est aussi acceptée pour examen et reste absente des packuments et routes de tarball.
Un modérateur du dépôt ou un administrateur système examine ensemble le manifeste immuable, les dist-tags et le tarball.

## Visibilité et équipes de paquet

Les paquets publics suivent la visibilité du dépôt. Un paquet privé doit être scoped et exige une appartenance explicite
ou un accès administrateur. L0 lit, L1 publie, L2 gère versions et métadonnées, L3 gère l'équipe et L4 possède le
paquet.
Une suppression ou rétrogradation ne peut pas retirer le dernier propriétaire L4.

Invitez des comptes RenoP existants depuis la page du paquet. Les invitations sont des actions durables du centre de
messages. Un paquet miroir n'a pas d'équipe locale, affiche son origine amont et reste en lecture seule.

## Dist-tags, dépréciation et retrait

Les commandes npm standard gèrent les tags de distribution et la dépréciation :

```bash
npm dist-tag add @example/library@1.0.0 stable
npm deprecate @example/library@1.0.0 "Use version 2"
npm unpublish @example/library@1.0.0
```

Le retrait crée une pierre tombale et supprime le tarball, sans rendre le numéro réutilisable. Supprimer le paquet
marque toutes ses versions et conserve le nom réservé.

## Miroirs amont

Un dépôt npm peut relayer un registre amont ordonné. Les noms exacts et règles `@scope/*` bornent le miroir. RenoP
limite les packuments, fusionne les rafraîchissements concurrents, remplace les URL de tarball par des URL locales et
retire du catalogue local les versions supprimées en amont. Les paquets découverts par miroir n'acceptent aucune
publication locale.
