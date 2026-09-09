---
title: Apparence des courriels
order: 8
category: Configuration
description: Préréglages, aperçu dans le navigateur et style visuel commun de RenoP
---

# Apparence des courriels

Les modèles reprennent les boutons bleus, cartes arrondies, fonds neutres et polices système de RenoP.
Le nom du site apparaît au-dessus du message, les codes ont un panneau distinct et les détails supplémentaires
sont séparés du texte principal. La version texte contient le même message et la même URL d’action.

## Choisir un préréglage

Définissez `mail.template_style`, ou choisissez le style sur la page de paramètres du courriel :

```yaml
mail:
  template_style: card
```

- `card` : carte standard avec un espacement confortable.
- `compact` : marges internes et titre plus petits.
- `notice` : carte standard avec une bordure supérieure ambrée.

Les identifiants de scènes et le choix de langue du compte restent identiques. Aucune image, police, feuille de
style ou script externe n’est nécessaire. Les couleurs intégrées définissent l’apparence de base ; les règles
adaptatives et sombres s’appliquent si le client les prend en charge. Le texte est échappé et les URL d’action
doivent appartenir à l’instance configurée.

## Aperçu et livraison

Les administrateurs peuvent prévisualiser une scène depuis les paramètres sans envoyer de message.
GET /api/settings/mail/templates/:scene?style=card renvoie le sujet, le HTML et le texte. La langue de l’aperçu suit
`Accept-Language` ; les notifications utilisent la langue enregistrée du destinataire, avec l’anglais par défaut.
L’aperçu emploie des exemples et ne contient pas d’identifiants stockés.

Les changements de modèle s’appliquent à l’entrée d’un nouveau message dans la file. Un message déjà en file
conserve son contenu rendu. Limites, réservations de crédit, contrôles de statut et interdiction de retenter une
soumission interrompue restent identiques. Voir [Livraison des courriels](mail.md) pour les fournisseurs et la file.
