---
title: Vue d’ensemble de la configuration
order: 1
category: Configuration
description: Fichiers, serveur, stockage, proxy, identité visuelle et mises à jour
---

# Vue d’ensemble de la configuration

RenoP lit `config.yaml` dans le répertoire de travail, sauf remplacement par `RENOP_CONFIG`. L’interface administrateur
utilise les mêmes structures validées et écrit les fichiers avec des permissions privées.

## Fichiers de configuration

| Fichier | Remplacement | Usage |
|:--------|:-------------|:------|
| `config.yaml` | `RENOP_CONFIG` | Serveur, base, aperçus, proxy, frontend, audit et mise à jour |
| `repositories.yaml` | `RENOP_REPOSITORIES` | Moteurs, visibilité, miroirs, politique Maven et S3 |
| `index.json` | `RENOP_INDEX` | Instantané de l’index de fichiers, reconstructible depuis le stockage |

Comptes, API Token, sessions, équipes, audit et messages sont en base, jamais dans YAML. Limitez la lecture des fichiers
de configuration au compte de service, car ils peuvent contenir des secrets.

## Schéma de `config.yaml`

### Stockage et aperçus de documentation

```yaml
storage_path: "storage"
enable_javadoc_preview: true
javadoc_extract_path: ""
max_javadoc_size_mb: 48
enable_cargodoc_preview: true
cargodoc_extract_path: ""
max_cargodoc_size_mb: 128
```

Un chemin vide utilise le cache de la plateforme. Les archives sont validées en chemin et taille avant exposition sous
`/javadoc` ou `/cargodoc`.

### Réseau et sécurité `server`

```yaml
server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains: ["localhost"]
  cors_origins: []
  enable_compression: false
  file_cache_size_mb: 16
  max_active_requests: 512
  trusted_proxies: []
  cdn_ip_header: "X-Forwarded-For"
  debug_mode: false
  gpg:
    key_servers: ["https://keys.openpgp.org", "https://keyserver.ubuntu.com"]
```

`domains` définit les hôtes publics et les hôtes CORS par défaut. `cors_origins` ajoute des origins exactes, hôtes ou
wildcards ; `*` autorise tout. Un en-tête IP transféré n’est fiable que si le pair direct appartient à
`trusted_proxies`. Hôte, port, TLS, compression, debug et certains caches exigent un redémarrage.

GitHub OAuth réside sous `server.github_oauth`; configurez Client ID et secret en écriture seule dans l’interface.

### Connexion `database`

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

Les pilotes sont `sqlite3` (ou `sqlite`), `mysql` et `postgres`. Voir
[Configuration de la base](./database.md).

### Routage sortant `proxy`

```yaml
proxy:
  selected: ""
  proxies:
    - name: "corp_proxy"
      url: "http://proxy.internal:8080"
      username: ""
      password: ""
```

La liste accepte jusqu’à 16 proxys HTTP, HTTPS ou SOCKS5. Voir
[Configuration du proxy](./outbound-proxy.md).

### Identité visuelle `frontend`

```yaml
frontend:
  id: "renop"
  title: "RenoP Package Registry"
  description: "Self-hosted package repository"
  organization_website: ""
  organization_logo: "/svg/logo.svg"
  background_url: ""
  icp_license: ""
  public_security_filing: ""
  legal_notice_url: ""
```

Les URL sont validées avant usage. L’arrière-plan doit respecter la politique WebP et de taille.

### Politique `updater`

```yaml
updater:
  channel: "release"
  mode: "manual"
```

`channel` vaut `release` ou `nightly`. `mode` vaut `manual`, `auto_check` ou `auto_install`. Les vérifications automatiques
sont fusionnées par le planificateur et leurs résultats sont envoyés aux administrateurs.
