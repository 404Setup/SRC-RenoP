---
title: Vérification en deux étapes
order: 4
category: Sécurité
description: Passkey comme second facteur, configuration de l’authentificateur, récupération et API privées
---

# Vérification en deux étapes

## Comportement de connexion

Configurez la vérification en deux étapes dans le panneau de sécurité du compte. Après la validation du mot de passe ou du compte tiers, RenoP exige un second facteur configuré avant de créer une session. Si un authentificateur et une Passkey secondaire sont tous deux activés, l’un ou l’autre peut terminer cette étape. Une Passkey secondaire ne permet pas de se connecter seule. Une Passkey principale peut aussi être suivie d’un code d’authentification.

La connexion par mot de passe ou Passkey principale renvoie `409` avec `X-Renop-Error-Code: MFA_REQUIRED` si un second facteur est nécessaire. GitHub et les autres fournisseurs OAuth configurés reviennent à `/account/login?mfa=1` avec la destination locale `return_to`. Le cookie privé HttpOnly `renop_mfa` autorise uniquement les endpoints de vérification en attente ; ce n’est pas une session. Les défis expirent après cinq minutes, sont à usage unique et sont perdus au redémarrage. Le processus conserve au plus 4 096 défis et huit par compte.

## Configurer un authentificateur

Choisissez **Configurer** à côté d’**Application d’authentification**. Scannez le code QR ou saisissez la clé manuellement avec des codes temporels, SHA-1, six chiffres et une période de 30 secondes. RenoP génère l’image QR sans transmettre ces informations à un service externe. Confirmez avec un code de l’application dans les cinq minutes. Une configuration non confirmée ne modifie pas la connexion.

Les codes suivent la [RFC 6238](https://www.rfc-editor.org/info/rfc6238/) avec une tolérance d’une période avant ou après l’heure du serveur. Synchronisez les horloges. Un compteur utilisé avec succès ne peut pas être réutilisé, y compris celui de la confirmation initiale ; attendez le code suivant si nécessaire. Cinq codes rejetés dans une fenêtre de cinq minutes bloquent les tentatives jusqu’à la fin de cette fenêtre pour ce compte. Un nouveau défi de connexion ne réinitialise pas cette limite.

## Passkey comme second facteur

Enregistrez une Passkey avant d’activer **Passkey comme second facteur**. Le mot de passe ou un compte tiers associé doit rester disponible comme méthode principale. RenoP empêche de désactiver la dernière méthode principale ou de supprimer la dernière Passkey secondaire tant que cette option est active. Désactivez l’option avant de retirer sa dernière Passkey. Les assertions secondaires exigent la vérification de l’utilisateur WebAuthn.

## API de vérification

Les corps JSON POST/PUT ci-dessous exigent `Content-Type: application/json` et sont limités à 4 096 octets. Les modifications du profil nécessitent le cookie actuel et une connexion datant de moins de cinq minutes ; sinon, la réponse est `MFA_REAUTH_REQUIRED`. Les changements réussis révoquent les autres sessions. La réponse de sécurité ajoute les booléens `totp_enabled` et `passkey_second_factor`, sans jamais inclure la clé de l’authentificateur.

| Méthode | Chemin | Corps JSON ou résultat |
|---|---|---|
| GET | `/api/auth/mfa` | `{totp,passkey,expires_at}` |
| DELETE | `/api/auth/mfa` | `204` |
| POST | `/api/auth/mfa/totp` | `{"code":"012345"}` |
| POST | `/api/auth/mfa/passkey/begin` | `{}` → `{options}` |
| POST | `/api/auth/mfa/passkey/finish` | `{credential}` → session |
| POST | `/api/auth/profile/mfa/totp/begin` | `{}` → `{id,secret,uri,qr,expires_at}` |
| POST | `/api/auth/profile/mfa/totp/confirm` | `{"id":"setup-id","code":"012345"}` → security |
| PUT | `/api/auth/profile/mfa` | `{"passkey_second_factor":true}` ou `{"totp_enabled":false}` → security |

```json
{"totp_enabled":true,"passkey_second_factor":true}
```

Ne placez pas les codes, images QR, clés de configuration ou cookies de défi dans des URL, des journaux ou le stockage du navigateur. Les erreurs de vérification utilisent `MFA_INVALID`, les pannes `MFA_UNAVAILABLE`, et une Passkey secondaire utilisée seule renvoie `MFA_PRIMARY_REQUIRED`. Une vérification réussie renvoie les détails de session en JSON et définit le cookie de session HttpOnly habituel.

## Récupération et exploitation

Quatre codes de récupération hors ligne inutilisés réinitialisent le mot de passe, retirent l’authentificateur, désactivent le mode Passkey secondaire et révoquent toutes les sessions dans une même transaction. Les Passkeys enregistrées restent associées et redeviennent utilisables pour la connexion principale. La récupération par email conserve les seconds facteurs. Protégez les codes hors ligne et reconfigurez la vérification après une récupération.

Les comptes avec un second facteur doivent utiliser des jetons API pour les clients de paquets et l’automatisation ; leur mot de passe est refusé par l’authentification des protocoles. Les jetons existants avec portée et les jetons d’envoi migrés gardent leurs permissions. Désactiver la vérification en deux étapes rétablit l’accès par mot de passe aux protocoles si la connexion par mot de passe est activée.

RenoP crée la clé privée de premier niveau `mfa_encryption_key` dans sa configuration au démarrage. Les clés d’authentificateur sont chiffrées avec AES-GCM et liées aux identifiants immuables des comptes. La clé de chiffrement est exclue des réponses des paramètres. Sauvegardez la configuration avec la base de données et conservez la clé lors d’une migration. Sa perte empêche la vérification par authentificateur ; utilisez les codes de récupération hors ligne pour rétablir l’accès.
