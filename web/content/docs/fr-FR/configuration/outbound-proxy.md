---
title: Proxy sortant
order: 4
category: Configuration
description: Proxys HTTP, HTTPS et SOCKS5 nommés et routage par miroir
---

# Configuration du proxy sortant

Configurez un proxy lorsque RenoP doit joindre Maven Central, crates.io, Docker, GitHub, GitLab ou les serveurs GPG via
une sortie contrôlée. Le processus partage des transports HTTP bornés par politique de routage.

## Schéma de configuration

```yaml
proxy:
  selected: "corp_http"
  proxies:
    - name: "corp_http"
      url: "http://10.0.0.1:8080"
      username: "proxy-user"
      password: "proxy-password"
    - name: "socks_proxy"
      url: "socks5://10.0.0.2:1080"
      username: ""
      password: ""
```

`selected` est la valeur globale ; vide signifie accès direct. Au plus 16 proxys nommés sont acceptés et leurs noms sont
uniques. Une URL utilise `http`, `https` ou `socks5`, contient hôte et port adaptés, mais aucun identifiant, chemin,
requête ou fragment. Placez les secrets dans `username` et `password` uniquement.

## Comportement du routage

| Sélecteur | Résultat |
|:----------|:---------|
| `""` | Hérite de `proxy.selected` |
| `direct` | Contourne tous les proxys |
| Nom de proxy | Utilise exactement ce proxy |

Une modification de sélection ou d’identifiants invalide les clients partagés concernés. Un nom inconnu est refusé au
lieu de basculer silencieusement en accès direct.

## Sélection par miroir

Chaque miroir peut remplacer la route globale avec `proxy` :

```yaml
repositories:
  releases:
    name: releases
    format: maven
    mirrors:
      - name: "maven-central"
        url: "https://repo1.maven.org/maven2"
        proxy: "corp_http"
      - name: "internal"
        url: "https://mirror.internal/maven"
        proxy: "direct"
      - name: "default-route"
        url: "https://plugins.gradle.org/m2"
        proxy: ""
```

Utilisez `direct` pour les services internes qui ne doivent jamais traverser le proxy. Ne placez aucun secret dans les
URL de miroirs ni dans les journaux.
