---
title: Maven & Gradle
order: 1
category: Guides
description: Verifying a publishing domain and configuring Maven and Gradle clients
---

# Maven & Gradle Client Configuration

Create a Maven repository, then create and verify the artifact's reverse-domain namespace from the account menu. The
domain and its L0-L4 team are global across Maven repositories. Reads use repository visibility; publication requires
both repository write permission and the domain's publication level.

For automation, prefer an expiring API token with `repository:read` and/or `repository:publish`. Use the account name as
the Basic username and the token as its password.

## Maven

### Dependency resolution (`pom.xml`)

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

Use a second repository entry for snapshots when required. `HIDDEN` repositories resolve by exact URL but do not appear
in discovery; `PRIVATE` repositories require credentials for reads.

### Deployment target (`pom.xml`)

```xml
<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>https://packages.example.com/releases</url>
    </repository>
</distributionManagement>
```

The `groupId` must fall under a verified domain controlled by the publisher. Classic and modern Maven layouts use the
same client URL and publication rules.

### Credentials (`~/.m2/settings.xml`)

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

The `<id>` must exactly match `pom.xml`. Keep credentials outside the project and secret-manager inject them in CI.

---

## Gradle

### Dependency resolution (`build.gradle.kts`)

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

### Publishing (`build.gradle.kts`)

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

Store `renopUser` and `renopToken` in user Gradle properties or CI secrets, not in source control.

## Javadoc viewer

When a version contains a valid `*-javadoc.jar` and preview is enabled, RenoP extracts it with path and size limits into
the sandboxed viewer.

Access URL: `https://packages.example.com/javadoc/{repo}/{group}/{artifact}/{version}/index.html`

Javadoc availability does not change artifact authorization. Signed status shown in the UI comes from the backend GPG
record, not from the archive filename.
