---
title: Introduction
order: 1
category: Premiers pas
description: Qu’est-ce que RenoP et à qui il s’adresse
---

# Introduction

RenoP est un **serveur Maven auto-hébergé** léger et rapidement déployable, pour les particuliers et les équipes.

Il se concentre sur :

- Une mise en route rapide avec des valeurs par défaut raisonnables
- Des dépôts release, snapshot et private
- Un proxy de miroirs Maven avec cache local
- Une petite interface web pour parcourir, téléverser, gérer les utilisateurs, les jetons et la santé

Si vous visez un **hébergement public**, ce n’est pas aujourd’hui le scénario principal de RenoP.

## Objectifs de conception

| Objectif            | Signification                                                               |
|---------------------|-----------------------------------------------------------------------------|
| Exploitation simple | Un binaire, des fichiers de config dans le répertoire de travail            |
| Natif Maven         | Disposition standard des dépôts et compatibilité clients                    |
| Transparence        | Pas de publicité, pas de télémétrie produit, édition communautaire gratuite |

## Étapes suivantes

1. [Installer](./install.md) une version stable ou preview
2. Suivre le [démarrage rapide](./quickstart.md)
3. Configurer un [client Maven](./maven-client.md)
4. Consulter la [configuration](../configuration/overview.md) pour un contrôle plus fin
