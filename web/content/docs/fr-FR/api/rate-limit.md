---
title: Limitation de débit
order: 9
category: API
---

# Limitation de débit et protection anti-anomalie

Le middleware global (`AnomalyMiddleware`) s’exécute sur chaque requête avant les handlers de route. Implémentation :
`middleware/anomaly.go`.

Trois contrôles indépendants s’appliquent dans l’ordre :

1. **Plafond de requêtes concurrentes** — à l’échelle du processus
2. **Ban après échecs d’auth** — par IP client
3. **Limitation de débit des requêtes anonymes** — par IP client (token bucket)

Quand une limite se déclenche, la réponse peut poser `Connection: close`.

## IP client

Les limites s’appuient sur l’IP client de `utils.ExtractIP` :

- Défaut : adresse peer (`c.IP()`).
- Si le peer est en loopback (`127.0.0.1` / `::1`, typique pour Caddy/nginx local) ou listé dans
  `server.trusted_proxies`, **et** que `server.cdn_ip_header` est défini, l’IP client est lue depuis cet en-tête (les
  en-têtes multi-valeurs sont parcourus de droite à gauche en ignorant les hops de confiance).

Une confiance proxy mal configurée peut regrouper beaucoup d’utilisateurs sous une IP (ou l’inverse). Voir la config
serveur dans [settings.md](./settings.md).

Pour Cloudflare → Caddy → RenoP, définissez `cdn_ip_header` sur `CF-Connecting-IP` ; les peers loopback n’ont pas besoin
d’entrée dans `trusted_proxies`.

## 1. Plafond de requêtes concurrentes

| Réglage                      | Défaut | Effet                                       |
|------------------------------|--------|---------------------------------------------|
| `server.max_active_requests` | `2000` | Max de requêtes in-flight dans le processus |

- Compté pour **toutes** les requêtes entrant dans le middleware (y compris authentifiées et statiques).
- Quand le compteur live dépasserait le plafond → **`503 Service Unavailable`**.
- `0` en config est normalisé au défaut (`2000`) au chargement ; pas de mode « illimité » via ce champ.

La limite de concurrence propre à Fiber est alignée sur cette valeur au démarrage.

## 2. Ban après échecs d’auth (par IP)

| Constante              | Valeur | Signification                             |
|------------------------|--------|-------------------------------------------|
| `MaxFailuresPerMinute` | `5`    | Seuil de compteur d’échecs avant ban      |
| Failure store TTL      | 5 min  | Fenêtre de vie BigCache `AnomalyFailures` |

**Ce qui compte comme échec**

Après le handler, si le statut final est **`401`** ou **`403`**, le compteur d’échecs par IP est incrémenté (compteur
little-endian 8 octets en cache).

**Comportement du ban**

- Si le compteur est déjà **≥ 5** au début d’une requête → **`403 Forbidden`** immédiatement (handler non exécuté).
- La réponse de ban est renvoyée *avant* `Next()`, donc elle n’incrémente **pas** le compteur à nouveau.
- Le TTL d’entrée est **5 minutes** depuis la dernière écriture (chaque échec enregistré rafraîchit l’entrée). Après
  expiration le compteur disparaît et l’IP peut réessayer.

**Portée**

S’applique à **tous** les chemins qui passent le skip frontend statique (ci-dessous), y compris les clients
authentifiés. Un client qui continue de recevoir 401/403 des handlers touchera ce ban quelle que soit la credential des
requêtes suivantes.

Note : le nom de constante dit « per minute », mais la fenêtre du store est **5 minutes** et le compteur n’augmente que
jusqu’à l’expiration de l’entrée.

## 3. Limitation de débit des requêtes anonymes (par IP)

Token bucket via `golang.org/x/time/rate`, un limiteur par IP (`GlobalIPLimiter`).

| Constante              | Valeur | Signification                                              |
|------------------------|--------|------------------------------------------------------------|
| `MaxRequestsPerMinute` | `100`  | Débit soutenu : 100 jetons / minute (~1 toutes les 600 ms) |
| `MaxRequestsBurst`     | `60`   | Capacité de burst (max de jetons tenus à la fois)          |

- **Qui est limité :** les requêtes **non** vérifiées comme authentifiées (voir ci-dessous).
- **Qui est exempt :** Session / Bearer / Basic vérifiés avec succès (ou GET/HEAD `?token=` résolvant une session ou un
  bearer valide).
- **En dépassement :** **`429 Too Many Requests`**.

### Ce que signifie « verified authenticated »

Même idée que les porteurs d’auth de production (voir [authentication.md](./authentication.md)) :

| Porteur                                           | Vérifié quand                                                     |
|---------------------------------------------------|-------------------------------------------------------------------|
| Cookie `renop_session` / `Authorization: Session` | La session existe et est dans le timeout d’inactivité             |
| `Authorization: Bearer <user>:<secret>`           | Nom + secret correspondent à un compte vivant                     |
| `Authorization: Bearer <upload-token>`            | Le jeton est indexé et non expiré                                 |
| `Authorization: Basic …`                          | Nom + password/secret correspondent                               |
| GET/HEAD `?token=`                                | Le jeton est un id de session valide ou un bearer comme ci-dessus |

Des identifiants présents mais **invalides** n’exemptent **pas** la requête ; ils consomment quand même des jetons de
rate-limit et peuvent augmenter le compteur d’échecs si le handler renvoie 401/403.

Les sessions inactives ne sont pas traitées comme authentifiées (timeout d’inactivité ≈ 7 jours ; voir docs d’auth).

### Cycle de vie du limiteur

| Comportement         | Détail                                                             |
|----------------------|--------------------------------------------------------------------|
| Clé                  | Chaîne IP client                                                   |
| Éviction idle        | Entrée inutilisée pendant **5 minutes** supprimée                  |
| Nettoyage périodique | Toutes les **5 minutes**                                           |
| Nettoyage d’urgence  | Quand les IP stockées dépassent **10 000**, un nettoyage part vite |

## Skip frontend statique

Pour **GET** et **HEAD** uniquement, ces chemins sautent le ban d’échecs d’auth et le rate limit anonyme (ils comptent
toujours dans le plafond concurrent) :

| Motif de chemin |
|-----------------|
| `/`             |
| `/index.html`   |
| `/assets/*`     |
| `/js/*`         |
| `/css/*`        |
| `/svg/*`        |

Les routes API, chemins de dépôt Maven et autres méthodes ne sont **pas** sautés.

## Codes de statut (ce middleware)

| Code | Quand                                                 |
|------|-------------------------------------------------------|
| 429  | IP anonyme au-delà du token-bucket rate limit         |
| 403  | IP bannie après trop de 401/403                       |
| 503  | Plafond de requêtes concurrentes du processus dépassé |

Corps généralement vides ; faites confiance au code de statut. Pas d’en-tête `Retry-After` aujourd’hui.

## Conseils clients

1. Préférez l’accès authentifié (cookie de session, Basic ou Bearer) pour le trafic volumineux ou scripté afin que le
   bucket anonyme par IP ne s’applique pas.
2. Évitez les boucles serrées de login / probe : **5** résultats **401/403** au niveau handler depuis une IP peuvent la
   bannir jusqu’à **~5 minutes**.
3. Derrière un reverse proxy ou CDN, configurez correctement `trusted_proxies` et `cdn_ip_header` pour limiter par vrai
   client, pas par proxy.
4. Sur **429**, faites un backoff (ex. exponentiel) avant de réessayer des appels non authentifiés.
5. Sur **503** de cette couche, l’instance est saturée ; réessayez plus tard ou scalez / augmentez `max_active_requests`
   si approprié.
