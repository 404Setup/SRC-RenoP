---
title: Checklist de mise en production
order: 4
category: Déploiement
description: Sécurité, persistance, proxy, dépôts, validation, supervision et retour arrière avant ouverture du service
---

# Checklist de mise en production

Utilisez cette checklist après le premier démarrage réussi et avant d’ouvrir le service aux clients de paquets ou à un
réseau non fiable. Un health check réussi est nécessaire, mais ne valide ni l’authentification, ni la base de données,
ni le stockage, les miroirs ou les règles de publication de bout en bout.

## Définir le périmètre du service

Documentez le nom public, l’écoute, le reverse proxy, la base, les backends de stockage et les responsables des dépôts.
Considérez RenoP comme un service coordonné. Une base externe ou un stockage compatible S3 remplace un composant local,
mais n’apporte pas à lui seul une coordination active-active sûre.

- Désignez un responsable d’exploitation et un contact sécurité.
- Notez la version de RenoP, les chemins de configuration, le compte de service, le répertoire de travail et le canal de
  mise à jour.
- Fixez la visibilité publique, masquée ou privée de chaque dépôt avant de diffuser la configuration cliente.
- Conservez l’interface de gestion et les endpoints de paquets sur la même origine HTTPS canonique, sauf si le proxy et
  les cookies ont été testés sur chaque origine publiée.

## Sécuriser l’initialisation et la récupération des comptes

`RENOP_DEFAULT_ADMIN_PASSWORD` ne sert qu’à la création initiale du compte `admin`. Si RenoP a généré le mot de passe,
récupérez-le dans le premier journal de démarrage puis remplacez-le immédiatement.

- Utilisez des comptes administrateurs nominatifs au lieu de partager `admin` au quotidien.
- Enregistrez une Passkey ou une autre méthode de connexion testée avant de désactiver le mot de passe.
- Générez les codes de récupération, conservez-les hors ligne et vérifiez l’adresse e-mail du compte.
- Créez un jeton API distinct et expirant pour chaque tâche CI, avec les seuls scopes et dépôts nécessaires.
- Placez les secrets de base, S3, OAuth, SMTP, signature et proxy dans un gestionnaire de secrets ou un environnement
  protégé.

## Publier le service en HTTPS

Lorsque le reverse proxy termine TLS, liez RenoP à une adresse loopback ou privée. Déclarez le nom public et uniquement
les proxys autorisés à fournir les en-têtes d’adresse cliente.

```yaml
server:
  host: "127.0.0.1"
  port: 3000
  domains:
    - "packages.example.com"
  trusted_proxies:
    - "127.0.0.1"
  cdn_ip_header: "X-Forwarded-For"
```

Le proxy doit préserver `Host`, le schéma d’origine et la chaîne d’adresses clientes. Désactivez la mise en tampon des
requêtes volumineuses, supprimez toute limite de corps involontaire et augmentez les délais de lecture/écriture pour les
couches d’image et gros artefacts. Ne faites pas confiance aux en-têtes transférés par n’importe quel client. Voir
[Reverse proxy](./reverse-proxy.md).

## Protéger la base et le stockage d’artefacts

Choisissez une base adaptée et validez-la par une véritable écriture authentifiée. Avec SQLite, utilisez un disque
durable et donnez au compte de service les droits sur le fichier et son répertoire. Avec une base externe, chiffrez le
transport si possible et limitez l’accès réseau à RenoP.

Pour chaque dépôt, vérifiez le backend local ou compatible S3, le bucket ou répertoire, le préfixe, les identifiants et
le mode de téléchargement. Une redirection présignée doit être accessible et approuvée depuis le réseau client ; le
streaming par RenoP garde le bucket privé mais fait transiter les artefacts par le service.

Préparez une sauvegarde regroupant configuration, définitions de dépôts, base de données et artefacts non
reconstructibles, puis répétez la restauration. Voir [Sauvegarde, restauration et migration](./backup-and-recovery.md).

## Définir les règles de dépôt et de publication

- Choisissez le bon format pour chaque dépôt ; les protocoles clients ne sont pas interchangeables.
- Vérifiez visibilité, droits de lecture et publication, équipes, propriété des namespaces, quotas et revue.
- Configurez les miroirs explicitement : délais, durée du cache, cache négatif et listes d’autorisation adaptées à
  l’amont.
- Vérifiez les domaines Maven, réservez les paquets npm et images Docker si nécessaire, et contrôlez les noms Cargo.
- Désignez les personnes capables d’approuver une publication ou un transfert de propriété.

## Valider avec les clients natifs

Testez le nom, les identifiants, le dépôt et le chemin de proxy réellement fournis aux utilisateurs. Pour chaque format,
réalisez au moins une lecture et une écriture autorisée. Si le cycle de vie est activé, testez aussi une suppression, un
yank, un archivage ou un changement de tag sur un paquet jetable.

```bash
curl --fail-with-body https://packages.example.com/api/status/health
```

Le corps attendu est `"UP"`.

Vérifiez également qu’un utilisateur anonyme ne lit pas un dépôt privé, qu’un jeton insuffisant est refusé, qu’un dépôt
masqué n’apparaît pas dans la découverte et qu’une publication interdite ne laisse aucun état partiel visible.

## Organiser l’exploitation et la supervision

- Surveillez le processus, la capacité, la base, l’expiration des certificats, la latence amont et les échecs répétés.
- Conservez les journaux hors du répertoire de travail et protégez-les comme des données potentiellement sensibles.
- Consultez régulièrement l’audit et les messages internes ; ils ne remplacent pas une alerte externe.
- Testez le canal stable ou nightly hors production avant d’automatiser les mises à jour.
- Définissez une fenêtre de maintenance et les personnes autorisées à révoquer sessions, jetons ou propriétés
  compromises.

## Checklist d’ouverture

- [ ] Le nom HTTPS canonique est accessible depuis tous les réseaux clients requis.
- [ ] Limites, buffering, délais et en-têtes du proxy ont été testés avec un gros envoi.
- [ ] Les méthodes de récupération administrateur et les codes hors ligne sont disponibles.
- [ ] La CI utilise des jetons limités et expirants, jamais un mot de passe personnel.
- [ ] La base et chaque backend ont réussi un test écriture/lecture/suppression.
- [ ] Visibilité, propriété, quotas, miroirs et revues ont été validés.
- [ ] Un dépôt privé reste privé par les URL directes et proxifiées.
- [ ] Les sauvegardes sont dans un autre domaine de panne et une restauration a réussi.
- [ ] Les alertes de capacité et de certificat ont des destinataires identifiés.
- [ ] Le binaire, la configuration et la procédure de retour arrière sont consignés.

## Préparer le retour arrière avant toute modification

Conservez le binaire précédent, la configuration et la sauvegarde base/stockage jusqu’à validation du nouveau service.
Restaurez une version applicative et un instantané de données compatibles ; ne supposez pas qu’une base migrée par une
version récente fonctionne toujours avec un ancien binaire. Consignez motif, horaires et dépôts touchés.

Pour les tests propres à chaque protocole, poursuivez avec les guides [Maven](../guides/maven-client.md),
[Cargo](../guides/cargo-registry.md), [npm](../guides/npm-registry.md) et
[Docker/OCI](../guides/docker-registry.md).
