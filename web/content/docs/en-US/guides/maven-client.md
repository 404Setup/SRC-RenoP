---
title: Maven & Gradle
order: 1
category: Guides
description: Configuring Maven, Gradle, and sbt for dependency resolution and publishing
---

# Maven & Gradle Client Configuration

This guide demonstrates how to configure Maven, Gradle, and other JVM build tools to resolve dependencies from and
deploy artifacts to RenoP.

## 1. Maven Configuration

### Dependency Resolution (`pom.xml`)

Add `<repositories>` to your `pom.xml`:

```xml
<repositories>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>http://localhost:3000/releases</url>
        <releases>
            <enabled>true</enabled>
        </releases>
        <snapshots>
            <enabled>false</enabled>
        </snapshots>
    </repository>
    <repository>
        <id>renop-snapshots</id>
        <name>RenoP Snapshots</name>
        <url>http://localhost:3000/snapshots</url>
        <releases>
            <enabled>false</enabled>
        </releases>
        <snapshots>
            <enabled>true</enabled>
        </snapshots>
    </repository>
</repositories>
```

### Deployment Target (`pom.xml`)

Configure `<distributionManagement>` for `mvn deploy`:

```xml
<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>http://localhost:3000/releases</url>
    </repository>
    <snapshotRepository>
        <id>renop-snapshots</id>
        <name>RenoP Snapshots</name>
        <url>http://localhost:3000/snapshots</url>
    </snapshotRepository>
</distributionManagement>
```

### Server Credentials (`~/.m2/settings.xml`)

Store server credentials in your local `settings.xml`. The `<id>` must match the repository IDs defined in your
`pom.xml`:

```xml
<settings>
    <servers>
        <server>
            <id>renop-releases</id>
            <username>admin</username>
            <password>your_password_or_token</password>
        </server>
        <server>
            <id>renop-snapshots</id>
            <username>admin</username>
            <password>your_password_or_token</password>
        </server>
    </servers>
</settings>
```

---

## 2. Gradle Configuration

### Kotlin DSL (`build.gradle.kts` / `settings.gradle.kts`)

```kotlin
repositories {
    maven {
        name = "renopReleases"
        url = uri("http://localhost:3000/releases")
        credentials {
            username = "admin"
            password = "your_password_or_token"
        }
    }
    maven {
        name = "renopSnapshots"
        url = uri("http://localhost:3000/snapshots")
        credentials {
            username = "admin"
            password = "your_password_or_token"
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
            val releasesRepoUrl = uri("http://localhost:3000/releases")
            val snapshotsRepoUrl = uri("http://localhost:3000/snapshots")
            url = if (version.toString().endsWith("SNAPSHOT")) snapshotsRepoUrl else releasesRepoUrl
            credentials {
                username = "admin"
                password = "your_password_or_token"
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

---

## 3. Javadoc Online Viewer

When an uploaded artifact contains a Javadoc JAR (e.g. `mylib-1.0.0-javadoc.jar`), RenoP automatically extracts the
archive and serves an interactive HTML preview:

Access URL:
`http://localhost:3000/javadoc/{repo}/{group}/{artifact}/{version}/index.html`
