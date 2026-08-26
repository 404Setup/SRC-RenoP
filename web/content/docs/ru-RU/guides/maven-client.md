---
title: Maven и Gradle
order: 1
category: Руководства
description: Проверка publishing domain и настройка клиентов Maven и Gradle
---

# Настройка клиентов Maven и Gradle

Создайте Maven repository, затем в меню аккаунта создайте и проверьте reverse-domain namespace артефакта. Domain и
команда L0-L4 глобальны для Maven-репозиториев. Чтение зависит от visibility; публикация требует repository write и
publication level домена.

Для automation используйте истекающий API Token с `repository:read` и/или `repository:publish`. Basic username — имя
аккаунта, password — Token.

## Maven

### Разрешение зависимостей (`pom.xml`)

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

При необходимости добавьте вторую запись для snapshots. `HIDDEN` разрешается по exact URL, но не обнаруживается;
`PRIVATE` требует credentials для чтения.

### Цель публикации (`pom.xml`)

```xml
<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>https://packages.example.com/releases</url>
    </repository>
</distributionManagement>
```

`groupId` должен входить в проверенный domain издателя. Classic и modern layout используют одинаковый URL и правила.

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

`<id>` точно совпадает с `pom.xml`. Держите credentials вне проекта и передавайте из CI secret manager.

---

## Gradle

### Разрешение зависимостей (`build.gradle.kts`)

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

### Публикация (`build.gradle.kts`)

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

Храните `renopUser` и `renopToken` в пользовательских Gradle properties или CI secrets, не в source control.

## Javadoc viewer

При наличии корректного `*-javadoc.jar` и включённом preview RenoP извлекает его с path/size limits в sandbox viewer.

URL: `https://packages.example.com/javadoc/{repo}/{group}/{artifact}/{version}/index.html`

Javadoc не меняет авторизацию. Signed state в UI поступает из backend GPG record, а не имени archive.
