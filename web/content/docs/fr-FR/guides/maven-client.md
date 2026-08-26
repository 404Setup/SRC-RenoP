---
title: Maven et Gradle
order: 1
category: Guides
description: Vérifier un domaine et configurer les clients Maven et Gradle
---

# Configuration des clients Maven et Gradle

Créez un dépôt Maven, puis créez et vérifiez le domaine inversé de l’artefact depuis le menu du compte. Le domaine et
son équipe L0-L4 sont globaux. La visibilité contrôle la lecture ; la publication exige écriture sur le dépôt et niveau
de publication du domaine.

Pour l’automatisation, préférez un API Token expirant avec `repository:read` et/ou `repository:publish`. Le nom du compte
est l’utilisateur Basic et le Token son mot de passe.

## Maven

### Résolution des dépendances (`pom.xml`)

```xml
<repositories>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>https://packages.example.com/releases</url>
        <releases><enabled>true</enabled></releases>
        <snapshots><enabled>false</enabled></snapshots>
    </repository>
</repositories>
```

Ajoutez une seconde entrée pour les snapshots. `HIDDEN` se résout par URL exacte sans être découvert ; `PRIVATE` exige
des identifiants pour la lecture.

### Cible de publication (`pom.xml`)

```xml
<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>https://packages.example.com/releases</url>
    </repository>
</distributionManagement>
```

Le `groupId` appartient à un domaine vérifié contrôlé par l’éditeur. Les présentations Maven classique et moderne
utilisent la même URL et les mêmes règles.

### Identifiants (`~/.m2/settings.xml`)

```xml
<settings>
    <servers>
        <server>
            <id>renop-releases</id>
            <username>alice</username>
            <password>rnp_pat_REDACTED</password>
        </server>
    </servers>
</settings>
```

`<id>` correspond exactement au `pom.xml`. Gardez les secrets hors du projet et injectez-les depuis le gestionnaire CI.

---

## Gradle

### Résolution (`build.gradle.kts`)

```kotlin
repositories {
    maven {
        name = "renopReleases"
        url = uri("https://packages.example.com/releases")
        credentials {
            username = providers.gradleProperty("renopUser").get()
            password = providers.gradleProperty("renopToken").get()
        }
    }
}
```

### Publication (`build.gradle.kts`)

```kotlin
plugins {
    `maven-publish`
}

publishing {
    repositories {
        maven {
            name = "renop"
            url = uri("https://packages.example.com/releases")
            credentials {
                username = providers.gradleProperty("renopUser").get()
                password = providers.gradleProperty("renopToken").get()
            }
        }
    }
    publications {
        create<MavenPublication>("mavenJava") {
            from(components["java"])
        }
    }
}
```

Stockez `renopUser` et `renopToken` dans les propriétés utilisateur Gradle ou les secrets CI, jamais dans Git.

## Aperçu Javadoc

Avec un `*-javadoc.jar` valide et l’aperçu activé, RenoP extrait sous limites de chemin et taille dans un viewer sandboxé.

URL : `https://packages.example.com/javadoc/{repo}/{group}/{artifact}/{version}/index.html`

L’aperçu ne modifie pas l’autorisation. L’état signé affiché vient du registre GPG backend, pas du nom de l’archive.
