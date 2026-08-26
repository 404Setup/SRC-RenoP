# Third-Party Notices

This document provides copyright and license notices for third-party software used by **RenoP**.

RenoP itself is licensed under the [Mozilla Public License 2.0](LICENSE). Third-party components remain under their own
licenses. This file is intended to satisfy attribution and notice obligations when RenoP is distributed in source or
binary form.

Versions below reflect the dependency set used to produce this notice (reviewed 2026-08-26). For an authoritative
build-time inventory, see `go.mod` / `go.sum`, `pnpm-lock.yaml`, and the workspace `package.json` files under
`internal/service/frontend/renop-html/`, `web/`, and `packages/`.

SPDX identifiers are used where the upstream package publishes a matching SPDX license expression. The Go table
identifies modules selected for the server build; the frontend table identifies the workspace's declared packages and
the lockfile remains authoritative for their transitive build dependencies.

---

## Design reference (not incorporated)

RenoP was developed with [Reposilite](https://github.com/dzikoysk/reposilite)
as a **design and product reference** for a small, self-hosted Maven repository. Reposilite code is **not** vendored or
redistributed by RenoP. Reposilite is available under its own license (Apache-2.0).

---

## Inventory

### A. Go toolchain, runtime, and standard library

RenoP binaries include runtime and standard-library code from the [404Setup Go fork](https://github.com/404Setup/go).
The exact fork release is selected from the Go version in `go.mod`. Prebuilt RenoP releases do not need an external Go
installation.

| Component                                          | Version                                             | SPDX         | Copyright / notices                                                                   |
|----------------------------------------------------|-----------------------------------------------------|--------------|---------------------------------------------------------------------------------------|
| [404Setup Go fork](https://github.com/404Setup/go) | `go.mod` directive (release resolved at build time) | BSD-3-Clause | Copyright 2009 The Go Authors; retains the Go project `LICENSE` and `PATENTS` notices |

### B. Go modules linked into the RenoP server binary

| Module                                | Version                            | SPDX                                       | Copyright / notices                                                                                            |
|---------------------------------------|------------------------------------|--------------------------------------------|----------------------------------------------------------------------------------------------------------------|
| `github.com/BurntSushi/toml`          | v1.6.0                             | MIT                                        | Copyright (c) 2013-2024 BurntSushi                                                                             |
| `filippo.io/edwards25519`             | v1.2.0                             | BSD-3-Clause                               | Copyright (c) 2019 The Go Authors                                                                              |
| `github.com/3JoB/unsafeConvert`       | v1.6.0                             | Apache-2.0                                 | Authors of unsafeConvert                                                                                       |
| `github.com/ProtonMail/go-crypto`     | v1.4.1                             | BSD-3-Clause                               | Copyright (c) The Go Authors and Proton AG                                                                     |
| `github.com/allegro/bigcache/v3`      | v3.2.0                             | Apache-2.0                                 | Copyright (c) 2019 Allegro                                                                                     |
| `github.com/andybalholm/brotli`       | v1.2.2                             | MIT                                        | Copyright (c) 2009, 2010, 2013-2016 by the Brotli Authors                                                      |
| `github.com/cespare/xxhash/v2`        | v2.3.0                             | MIT                                        | Copyright (c) 2016 Caleb Spare                                                                                 |
| `github.com/cloudflare/circl`         | v1.6.5                             | BSD-3-Clause                               | Copyright (c) 2020 Cloudflare, Inc.                                                                            |
| `github.com/dustin/go-humanize`       | v1.0.1                             | MIT                                        | Copyright (c) 2005-2008 Dustin Sallings                                                                        |
| `github.com/fsnotify/fsnotify`        | v1.10.1                            | BSD-3-Clause                               | Copyright © 2012 The Go Authors; Copyright © fsnotify Authors                                                  |
| `github.com/fxamacker/cbor/v2`        | v2.9.3                             | MIT                                        | Copyright (c) 2019 Faye Amacker                                                                                |
| `github.com/go-ole/go-ole`            | v1.3.0                             | MIT                                        | Copyright © 2013-2017 Yasuhiro Matsumoto                                                                       |
| `github.com/go-sql-driver/mysql`      | v1.10.0                            | MPL-2.0                                    | Copyright (c) 2013 The Go-MySQL-Driver Authors                                                                 |
| `github.com/go-viper/mapstructure/v2` | v2.5.0                             | MIT                                        | Copyright (c) 2013 Mitchell Hashimoto                                                                          |
| `github.com/go-webauthn/webauthn`     | v0.17.4                            | BSD-3-Clause                               | Copyright (c) 2019-present WebAuthn Authors                                                                    |
| `github.com/go-webauthn/x`            | v0.3.0                             | BSD-3-Clause                               | Copyright (c) 2023 WebAuthn Authors                                                                            |
| `github.com/goccy/go-json`            | v0.10.6                            | MIT                                        | Copyright (c) 2020 Masaaki Goshima                                                                             |
| `github.com/gofiber/fiber/v3`         | v3.5.0                             | MIT                                        | Copyright (c) 2019-present Fenny and Contributors                                                              |
| `github.com/gofiber/schema`           | v1.8.4                             | BSD-3-Clause                               | Copyright (c) 2023 The Gorilla Authors                                                                         |
| `github.com/gofiber/utils/v2`         | v2.4.1                             | MIT                                        | Copyright (c) 2020-present Fenny and Contributors                                                              |
| `github.com/golang-jwt/jwt/v5`        | v5.3.1                             | MIT                                        | Copyright (c) 2020-present Go Language JWT Authors                                                             |
| `github.com/google/go-tpm`            | v0.9.8                             | Apache-2.0                                 | Copyright (c) Google LLC                                                                                       |
| `github.com/google/uuid`              | v1.6.0                             | BSD-3-Clause                               | Copyright (c) 2009, 2014 Google Inc.                                                                           |
| `github.com/jackc/pgpassfile`         | v1.0.0                             | MIT                                        | Copyright (c) 2019 Jack Christensen                                                                            |
| `github.com/jackc/pgservicefile`      | v0.0.0-20240606120523-5a60cdf6a761 | MIT                                        | Copyright (c) 2019 Jack Christensen                                                                            |
| `github.com/jackc/pgx/v5`             | v5.10.0                            | MIT                                        | Copyright (c) 2013-2024 Jack Christensen                                                                       |
| `github.com/jackc/puddle/v2`          | v2.2.2                             | MIT                                        | Copyright (c) 2019 Jack Christensen                                                                            |
| `github.com/klauspost/compress`       | v1.19.2                            | BSD-3-Clause (+ Apache-2.0 for some files) | Copyright (c) 2012 The Go Authors; Copyright (c) 2019 Klaus Post                                               |
| `github.com/klauspost/cpuid/v2`       | v2.4.0                             | MIT                                        | Copyright (c) 2015 Klaus Post                                                                                  |
| `github.com/klauspost/crc32`          | v1.3.0                             | BSD-3-Clause                               | Copyright (c) 2012 The Go Authors                                                                              |
| `github.com/llxisdsh/pb`              | v1.5.25                            | MIT                                        | Copyright (c) 2025 llxisdsh                                                                                    |
| `github.com/mattn/go-colorable`       | v0.1.15                            | MIT                                        | Copyright (c) 2016 Yasuhiro Matsumoto                                                                          |
| `github.com/mattn/go-isatty`          | v0.0.24                            | MIT                                        | Copyright (c) Yasuhiro MATSUMOTO                                                                               |
| `github.com/minio/crc64nvme`          | v1.1.1                             | Apache-2.0                                 | Copyright (c) 2025 Minio Inc.                                                                                  |
| `github.com/minio/md5-simd`           | v1.1.2                             | Apache-2.0                                 | Copyright (c) 2020 MinIO Inc.                                                                                  |
| `github.com/minio/minio-go/v7`        | v7.3.0                             | Apache-2.0                                 | See [Apache NOTICE excerpts](#apache-notice-excerpts)                                                          |
| `github.com/molecule-man/go-brrr`     | v1.0.1                             | MIT                                        | Copyright (c) 2026 Andrii Berezhynskyi                                                                       |
| `github.com/ncruces/go-strftime`      | v1.0.0                             | MIT                                        | Copyright (c) 2022 Nuno Cruces                                                                                 |
| `github.com/philhofer/fwd`            | v1.2.0                             | MIT                                        | Copyright (c) 2014-2015 Philip Hofer                                                                           |
| `github.com/remyoudompheng/bigfft`    | v0.0.0-20230129092748-24d4a6f8daec | BSD-2-Clause                               | Copyright (c) 2012 Rémi Oudompheng                                                                             |
| `github.com/rs/xid`                   | v1.6.0                             | MIT                                        | Copyright (c) 2015 Olivier Poitrey                                                                             |
| `github.com/shirou/gopsutil/v3`       | v3.24.5                            | BSD-3-Clause                               | Copyright (c) 2014 WAKAYAMA Shirou                                                                             |
| `github.com/tinylib/msgp`             | v1.6.4                             | MIT                                        | Copyright (c) 2014 Philip Hofer; portions Copyright (c) 2009 The Go Authors                                    |
| `github.com/valyala/bytebufferpool`   | v1.0.0                             | MIT                                        | Copyright (c) 2016 Aliaksandr Valialkin, VertaMedia                                                            |
| `github.com/valyala/fasthttp`         | v1.73.0                            | MIT                                        | Copyright (c) 2015-present Aliaksandr Valialkin, VertaMedia, Kirill Danshin, Erik Dubbelboer, FastHTTP Authors |
| `github.com/x448/float16`             | v0.8.4                             | MIT                                        | Copyright (c) 2019 Faye Amacker                                                                                |
| `github.com/yusufpapurcu/wmi`         | v1.2.4                             | MIT                                        | Copyright (c) 2013 Stack Exchange                                                                              |
| `github.com/zeebo/xxh3`               | v1.1.0                             | BSD-2-Clause                               | Copyright (c) 2012-2014 Yann Collet; Copyright (c) 2019 Jeff Wendling                                          |
| `go.yaml.in/yaml/v3`                  | v3.0.5                             | MIT AND Apache-2.0                         | Copyright (c) 2006-2011 Kirill Simonov (libyaml ports); Copyright (c) 2011-2019 Canonical Ltd                  |
| `golang.org/x/crypto`                 | v0.55.0                            | BSD-3-Clause                               | Copyright 2009 The Go Authors                                                                                  |
| `golang.org/x/mod`                    | v0.40.0                            | BSD-3-Clause                               | Copyright 2009 The Go Authors                                                                                  |
| `golang.org/x/net`                    | v0.58.0                            | BSD-3-Clause                               | Copyright 2009 The Go Authors                                                                                  |
| `golang.org/x/sync`                   | v0.22.0                            | BSD-3-Clause                               | Copyright 2009 The Go Authors                                                                                  |
| `golang.org/x/sys`                    | v0.47.0                            | BSD-3-Clause                               | Copyright 2009 The Go Authors                                                                                  |
| `golang.org/x/text`                   | v0.41.0                            | BSD-3-Clause                               | Copyright 2009 The Go Authors                                                                                  |
| `golang.org/x/time`                   | v0.15.0                            | BSD-3-Clause                               | Copyright 2009 The Go Authors                                                                                  |
| `google.golang.org/protobuf`          | v1.36.12                           | BSD-3-Clause                               | Copyright (c) 2018 The Go Authors                                                                              |
| `gopkg.in/ini.v1`                     | v1.67.3                            | Apache-2.0                                 | Copyright 2014– Unknwon and contributors                                                                       |
| `modernc.org/libc`                    | v1.75.4                            | BSD-3-Clause                               | Copyright (c) 2017 The Libc Authors                                                                            |
| `modernc.org/mathutil`                | v1.7.1                             | BSD-3-Clause                               | Copyright (c) 2017 The Mathutil Authors                                                                        |
| `modernc.org/memory`                  | v1.12.1                            | BSD-3-Clause                               | Copyright (c) 2017 The Memory Authors                                                                          |
| `modernc.org/sqlite`                  | v1.57.0                            | BSD-3-Clause                               | Copyright (c) 2017 The Sqlite Authors                                                                          |

Platform-specific transitive modules (via `gopsutil` and related code), which may be compiled into some targets:

| Module                             | Version                            | SPDX         | Copyright / notices                    |
|------------------------------------|------------------------------------|--------------|----------------------------------------|
| `github.com/lufia/plan9stats`      | v0.0.0-20260802145828-341c2f0c90b5 | BSD-3-Clause | Copyright (c) 2019 KADOTA, Kyohei      |
| `github.com/power-devops/perfstat` | v0.0.0-20260805114148-88456608a4f6 | MIT          | Copyright (c) 2020 Power DevOps        |
| `github.com/shoenig/go-m1cpu`      | v0.2.2                             | MPL-2.0      | Copyright (c) The M1CPU Authors        |
| `github.com/tklauser/go-sysconf`   | v0.4.0                             | BSD-3-Clause | Copyright (c) 2018-2022 Tobias Klauser |
| `github.com/tklauser/numcpus`      | v0.12.0                            | Apache-2.0   | Copyright 2018 Tobias Klauser          |

### C. Test-only Go modules (not required for binary distribution)

These are pulled by the test dependency graph. They are not part of release binaries, but remain third-party software in
the source tree:

| Module                          | Version | SPDX               | Copyright / notices                                              |
|---------------------------------|---------|--------------------|------------------------------------------------------------------|
| `github.com/stretchr/testify`   | v1.12.1 | MIT                | Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors |
| `github.com/davecgh/go-spew`    | v1.1.1  | ISC                | Copyright (c) 2012-2016 Dave Collins                             |
| `github.com/pmezard/go-difflib` | v1.0.0  | BSD-3-Clause       | Copyright (c) 2013 Patrick Mezard                                |
| `gopkg.in/yaml.v3`              | v3.0.1  | MIT AND Apache-2.0 | Same dual-license scheme as `go.yaml.in/yaml/v3`                 |

### D. Frontend / website dependencies and assets

The management UI is embedded into the server binary after bundling. Website packages under `web/` are used for the
marketing site and documentation, not the server binary.

| Package                                                  | Version | SPDX         | Role                                         |
|----------------------------------------------------------|---------|--------------|----------------------------------------------|
| [Feather Icons](https://github.com/feathericons/feather) | —       | MIT          | Interface icon design reference / assets     |
| `rolldown`                                               | 1.2.5   | MIT          | JS/CSS bundler (build-time)                  |
| `lightningcss`                                           | 1.33.0  | MPL-2.0      | CSS transformer (build-time)                 |
| `protobufjs`                                             | 8.7.2   | BSD-3-Clause | Frontend protobuf runtime / codegen          |
| `long`                                                   | 5.3.2   | Apache-2.0   | Transitive protobufjs runtime dependency     |
| `protobufjs-cli`                                         | 2.6.2   | BSD-3-Clause | Frontend protobuf codegen (dev)              |
| `marked`                                                 | 18.0.10 | MIT          | Markdown rendering (frontend docs & website) |
| `brotli-compress`                                        | 2.2.2   | Apache-2.0 AND MIT | Pure-JavaScript Brotli decompression in the website conversion worker; decoder copyright 2017 Google Inc. |
| `fflate`                                                 | 0.8.3   | MIT          | Browser-side legacy ZIP generation; copyright (c) 2026 Arjun Barrett |

---

## Special license notes

### Mozilla Public License 2.0 components

The following third-party works are under **MPL-2.0** (file-level copyleft). RenoP does not relicense them. Source for
these components is available from their upstream repositories (and, for Go modules, via the Go module proxy /
`go.mod` versions listed above):

- `github.com/go-sql-driver/mysql` (database driver for MySQL)
- `github.com/shoenig/go-m1cpu` (may be linked on some platforms via gopsutil)
- `lightningcss` (CSS build tool; not linked as a Go library)

RenoP’s own source is also MPL-2.0; see [LICENSE](LICENSE).

### Apache License 2.0 notice retention

Apache-2.0 requires that redistributions retain applicable copyright, patent, trademark, and attribution notices, and
any `NOTICE` file contents. Relevant upstream `NOTICE` text is reproduced below.

### Dual-licensed YAML (`go.yaml.in/yaml/v3`, `gopkg.in/yaml.v3`)

Portions ported from libyaml remain under the **MIT** license (Copyright (c) 2006-2011 Kirill Simonov). Remaining files
are under **Apache-2.0** (Copyright (c) 2011-2019 Canonical Ltd). Both notices are retained.

### `klauspost/compress`

Primarily **BSD-3-Clause**. Some subpaths (for example `gzhttp/*`) are under **Apache-2.0**. See the package’s own
`LICENSE` file for the full split.

### 404Setup Go fork

The custom Go distribution used to build RenoP is maintained at
<https://github.com/404Setup/go>. It is a modified version of the Go project and retains the upstream
[BSD-3-Clause license](https://github.com/404Setup/go/blob/master/LICENSE) and
[additional patent grant](https://github.com/404Setup/go/blob/master/PATENTS). Its copyright notice is:

```text
Copyright 2009 The Go Authors.
```

The BSD-3-Clause terms are reproduced in [License texts](#bsd-3-clause-license). The additional patent grant is
available with the fork source.

---

## Apache NOTICE excerpts

### MinIO Go Client (`github.com/minio/minio-go`)

```text
MinIO Cloud Storage, (C) 2014-2020 MinIO, Inc.

This product includes software developed at MinIO, Inc.
(https://min.io/).

The MinIO project contains unmodified/modified subcomponents too with
separate copyright notices and license terms. Your use of the source
code for these subcomponents is subject to the terms and conditions
of Apache License Version 2.0
```

### go-yaml (`go.yaml.in/yaml/v3` / `gopkg.in/yaml.v3`)

```text
Copyright 2011-2016 Canonical Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

---

## License texts

The following are standard license texts that apply to the packages listed above. Package-specific copyright lines
appear in the inventory tables.

### MIT License

```text
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### Apache License 2.0

Full text: <https://www.apache.org/licenses/LICENSE-2.0>

```text
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS
```

### BSD 2-Clause License

```text
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

### BSD 3-Clause License

```text
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

### ISC License

```text
Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
```

### Mozilla Public License 2.0

Full text: [LICENSE](LICENSE) (RenoP’s project license is the same SPDX identifier)
or <https://mozilla.org/MPL/2.0/>.

---

## Maintaining this file

When adding, removing, or upgrading dependencies:

1. Update `go.mod` / frontend lockfiles as usual.
2. Refresh the inventory tables (module path, version, SPDX, copyright).
3. If an Apache-2.0 package ships a `NOTICE` file, copy any new notice text
   into [Apache NOTICE excerpts](#apache-notice-excerpts).
4. Ship this file alongside raw Brotli binaries and include it in browser-generated legacy ZIP packages (see `build.ps1`).

This document is provided for compliance and attribution. It does not grant any license to RenoP
beyond [LICENSE](LICENSE), and it does not modify the terms of any third-party license.
