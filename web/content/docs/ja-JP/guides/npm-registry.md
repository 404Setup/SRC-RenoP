---
title: npm レジストリ
order: 4
category: ガイド
description: パッケージを予約し、npm、pnpm、Yarn、Bun から RenoP を利用する
---

# npm レジストリガイド

`npm` 形式のリポジトリを作成し、公開前にリポジトリ画面から各パッケージを予約します。RenoP はクライアント
による暗黙の名前作成を許可しません。例ではリポジトリ `javascript` とパッケージ `@example/library` を使います。

## クライアント設定

リポジトリの読み取り・公開 scope を持つ期限付き API Token を作成します。ライフサイクルやチーム管理の scope
は自動化に必要な場合だけ追加してください。専用レジストリではプロジェクトまたはユーザーの `.npmrc` に記述します。

```ini
registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

特定 scope だけを RenoP に向ける場合は、既定レジストリを維持して scope を個別に設定します。

```ini
@example:registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

信頼済みのローカル開発環境以外では HTTPS を使います。自動化には API Token を推奨し、アカウントパスワード
は標準パッケージプロトコルの認証だけに使用します。

## パッケージの準備と公開

予約名と `package.json` の `name` は完全一致が必要です。バージョンは SemVer で、公開成功後は変更できません。

```json
{
  "name": "@example/library",
  "version": "1.0.0",
  "description": "Example library",
  "publishConfig": {
    "registry": "https://packages.example.com/javascript/"
  }
}
```

互換クライアントから公開・インストールできます。

```bash
npm publish
npm install @example/library
pnpm add @example/library
yarn add @example/library
bun add @example/library
```

RenoP はサイズ制限付き gzip tarball を検証し、`package/package.json` と要求の一致を確認し、npm 互換の SHA-1
と SHA-512 integrity を計算します。すべての検証に成功した後だけアーカイブを確定します。

## 可視性とパッケージチーム

公開パッケージはリポジトリ可視性に従います。非公開パッケージは scoped 名が必須で、明示的なメンバーまたは
管理者だけがアクセスできます。L0 は読み取り、L1 は公開、L2 はバージョンとメタデータ、L3 はチーム管理、L4
は所有権です。最後の L4 所有者を失う削除・降格は拒否されます。

既存 RenoP アカウントをパッケージ画面から招待します。招待はメッセージセンターの永続アクションです。ミラー
パッケージにはローカルチームがなく、上流の出所を表示し、常に読み取り専用です。

## Dist-tag、非推奨化、公開取消

標準 npm コマンドで配布タグと非推奨メタデータを管理します。

```bash
npm dist-tag add @example/library@1.0.0 stable
npm deprecate @example/library@1.0.0 "Use version 2"
npm unpublish @example/library@1.0.0
```

公開取消はバージョンを tombstone 化して tarball を削除しますが、番号は再利用できません。パッケージ削除は全
バージョンを tombstone 化し、名前の予約を維持します。

## 上流ミラー

npm リポジトリは順序付き上流レジストリをプロキシできます。完全名と `@scope/*` で対象を制限します。RenoP
は packument サイズを制限し、同時更新を統合し、tarball URL をローカル URL に置換し、上流から消えた
バージョンをローカル catalog から除外します。ミラーで発見したパッケージにはローカル公開できません。
