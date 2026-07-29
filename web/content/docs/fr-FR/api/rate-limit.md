---
title: Limitation de débit
order: 9
category: API
---

# Limitation de débit

Le middleware global `AnomalyMiddleware` (`middleware/anomaly.go`) s’exécute avant les handlers de route.

Contrôles, dans l’ordre :

1. Plafond de requêtes concurrentes (processus)
2. Bannissement après échecs d’authentification (par IP client)
3. Limitation de débit des requêtes anonymes (par IP client, token bucket)

Lorsqu’une limite s’applique, la réponse peut définir `Connection: close`.

## IP client

Les limites sont indexées par l’IP client fournie par `utils.ExtractIP` :

- Par défaut : adresse peer (`c.IP()`).
- Si le peer est en loopback (`127.0.0.1` / `::1`) ou listé dans `server.trusted_proxies`, et que `server.cdn_ip_header`
  est défini, l’IP client est lue depuis cet en-tête. Les en-têtes multi-valeurs sont parcourus de droite à gauche ; les
  hops de confiance sont ignorés.

Une configuration incorrecte de confiance proxy peut faire correspondre plusieurs clients à une seule IP, ou l’inverse.
Voir [settings.md](./settings.md).

Pour Cloudflare → Caddy → RenoP, définir `cdn_ip_header` à `CF-Connecting-IP`. Les peers en loopback n’ont pas besoin
d’entrée dans `trusted_proxies`.

## 1. Plafond de requêtes concurrentes

| Réglage                      | Défaut | Effet                             |
|------------------------------|--------|-----------------------------------|
| `server.max_active_requests` | `2000` | Nombre maximal de requêtes en vol |

- Toute requête entrant dans le middleware est comptée, y compris les requêtes authentifiées et statiques.
- Si le compteur en direct dépasserait le plafond, le middleware renvoie **`503 Service Unavailable`**.
- Une valeur configurée de `0` est normalisée à `2000` au chargement. Ce champ n’offre pas de mode illimité.

La limite de concurrence de Fiber est alignée sur cette valeur au démarrage du serveur.

## 2. Bannissement après échecs d’auth (par IP)

| Constante              | Valeur | Signification                             |
|------------------------|--------|-------------------------------------------|
| `MaxFailuresPerMinute` | `5`    | Seuil de failures avant ban               |
| Failure store TTL      | 5 min  | Fenêtre de vie du cache `AnomalyFailures` |

**Compté comme failure :** après le retour du handler, si le statut final est **`401`** ou **`403`**, le compteur de
failures par IP est incrémenté (valeur little-endian sur 8 octets en cache).

**Comportement de ban :**

- Si le compteur est déjà **≥ 5** au début de la requête, le middleware renvoie **`403 Forbidden`** immédiatement sans
  exécuter le handler.
- La réponse de ban est produite avant `Next()`, elle n’incrémente donc pas le compteur.
- Le TTL de l’entrée est de **5 minutes** depuis la dernière écriture ; chaque failure enregistrée rafraîchit l’entrée.
  Après expiration, le compteur est supprimé.

Le ban s’applique à tous les chemins hors liste de skip du frontend statique ci-dessous, y compris les clients déjà
authentifiés. Des 401/403 répétés depuis les handlers bannissent l’IP indépendamment des credentials des requêtes
suivantes.

Note : le nom de la constante mentionne « per minute », mais la fenêtre de stockage est de **5 minutes**. Le compteur
n’augmente que jusqu’à l’expiration de l’entrée.

## 3. Limitation de débit anonyme (par IP)

Token bucket via `golang.org/x/time/rate`, un limiteur par IP (`GlobalIPLimiter`).

| Constante              | Valeur | Signification                                                |
|------------------------|--------|--------------------------------------------------------------|
| `MaxRequestsPerMinute` | `100`  | Débit soutenu : 100 tokens par minute (~1 toutes les 600 ms) |
| `MaxRequestsBurst`     | `60`   | Capacité de burst (maximum de tokens détenus à la fois)      |

- **Limitées :** les requêtes non vérifiées comme authentifiées (voir ci-dessous).
- **Exemptées :** Session, Bearer ou Basic validés avec succès, ou GET/HEAD `?token=` résolu en session ou bearer
  valide.
- **En dépassement :** **`429 Too Many Requests`**.

### Authentification requise pour l’exemption

Les porteurs correspondent à l’auth de production ([authentication.md](./authentication.md)) :

| Porteur                                           | Vérifié lorsque                                 |
|---------------------------------------------------|-------------------------------------------------|
| Cookie `renop_session` / `Authorization: Session` | La session existe et est dans le timeout d’idle |
| `Authorization: Bearer <user>:<secret>`           | Username et secret correspondent à un compte    |
| `Authorization: Bearer <upload-token>`            | Le token est indexé et non expiré               |
| `Authorization: Basic …`                          | Username et password/secret correspondent       |
| GET/HEAD `?token=`                                | Token = session id ou bearer valides            |

Des credentials invalides n’accordent pas d’exemption. Ces requêtes consomment toujours des tokens de rate limit, et un
401/403 du handler incrémente toujours le compteur de failures.

Les sessions expirées par idle ne sont pas considérées comme authentifiées (timeout d’idle d’environ 7 jours ; voir la
documentation d’authentification).

### Cycle de vie des limiteurs

| Comportement         | Détail                                                |
|----------------------|-------------------------------------------------------|
| Clé                  | Chaîne IP client                                      |
| Éviction idle        | Entrée inutilisée pendant **5 minutes** supprimée     |
| Nettoyage périodique | Toutes les **5 minutes**                              |
| Nettoyage d’urgence  | Au-delà de **10 000** IP stockées, nettoyage immédiat |

## Skip du frontend statique

Pour **GET** et **HEAD** uniquement, les chemins suivants ignorent le ban d’échecs d’auth et la limitation de débit
anonyme. Ils restent comptés pour le plafond concurrent :

| Chemin        |
|---------------|
| `/`           |
| `/index.html` |
| `/assets/*`   |
| `/js/*`       |
| `/css/*`      |
| `/svg/*`      |

Les routes API, les chemins de dépôt Maven et les autres méthodes HTTP ne sont pas ignorés.

## Codes de statut

| Code | Condition                                             |
|------|-------------------------------------------------------|
| 429  | IP anonyme au-delà de la limitation token-bucket      |
| 403  | IP bannie après trop de réponses 401/403              |
| 503  | Plafond de requêtes concurrentes du processus dépassé |

Les corps de réponse sont généralement vides. Il n’y a pas d’en-tête `Retry-After`.
