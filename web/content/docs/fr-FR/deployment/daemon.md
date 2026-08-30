---
title: Gestion du service système
order: 1
category: Déploiement
description: Enregistrer RenoP comme service natif avec --install et --uninstall
---

# Gestion du service système

RenoP sait s’enregistrer comme service démarré automatiquement, sans wrapper tiers.

## Commandes

```bash
# Register and start as a system service
./renop --install

# Configure a local Caddy reverse proxy
./renop --install-caddy --hostname renop.example.com

# Stop and remove the system service
./renop --uninstall

# View CLI help
./renop --help
```

`--install` enregistre le chemin absolu du binaire et utilise son dossier comme répertoire de travail. Exécutez la
commande depuis le répertoire définitif, par exemple `/opt/renop` ou `C:\Program Files\RenoP`.

## Plateformes

| Système             | Gestionnaire | Comportement                                                        |
|:--------------------|:-------------|:--------------------------------------------------------------------|
| **Windows**         | SCM          | Service `RenoP`, démarrage automatique, visible dans `services.msc` |
| **Linux (systemd)** | systemd      | Crée `/etc/systemd/system/renop.service` et active le service       |
| **Linux (OpenRC)**  | OpenRC       | Crée `/etc/init.d/renop` et l’ajoute au niveau par défaut           |
| **macOS**           | launchd      | Crée et charge `/Library/LaunchDaemons/one.pkg.renop.plist`         |
| **BSD**             | rc.d         | Génère le script de service rc.d adapté                             |

L’installation et la suppression exigent les privilèges système. Déplacez ou remplacez le binaire uniquement selon la
procédure de mise à jour, afin de conserver le chemin enregistré.

## Opérations courantes

### Linux (systemd)

```bash
systemctl status renop    # Check service status
systemctl restart renop   # Restart service
journalctl -u renop -f    # Tail real-time logs
```

### Windows (PowerShell)

```powershell
Get-Service RenoP         # Check service status
Restart-Service RenoP     # Restart service
```
