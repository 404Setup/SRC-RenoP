---
title: Limitation de débit et défense
order: 12
category: Référence API
description: Limitation de débit, détection d’anomalies et protection des adresses IP
---

# Limitation de débit et défense

RenoP combine plusieurs limites et contrôles d’anomalies afin de réduire les attaques par force brute, les dénis de
service et l’extraction automatisée excessive.

## Limites anonymes

Les requêtes non authentifiées sont évaluées par adresse IP avec une fenêtre glissante et un seau de jetons :

- les téléchargements publics disposent d’une marge élevée ;
- la recherche et les métadonnées ont des limites plus strictes et renvoient `429 Too Many Requests` en cas de
  dépassement.

## Échecs d’authentification et bannissement

- Les identifiants invalides ou tentatives de connexion renvoyant `401 Unauthorized` ou `403 Forbidden` sont comptés par IP.
- Après 10 échecs, les requêtes suivantes renvoient `403 Forbidden`. Le compteur expire cinq minutes après le dernier
  échec comptabilisé ; les requêtes bloquées ne prolongent pas ce délai.
- Les demandes anonymes de permission et les refus pour une session valide ne sont pas comptés. Les pages et ressources
  statiques restent accessibles.

## Concurrence (`max_active_requests`)

Configurez `server.max_active_requests` dans `config.yaml` (512 par défaut).

- Lorsque le nombre de requêtes actives atteint cette limite, les nouvelles requêtes reçoivent
  `503 Service Unavailable`.

## Proxys de confiance

Derrière un reverse proxy ou un CDN, configurez `server.trusted_proxies` et `server.cdn_ip_header` dans `config.yaml`.
RenoP utilise alors l’adresse réelle validée du client pour les limites, jamais un en-tête transmis par une source non
approuvée.
