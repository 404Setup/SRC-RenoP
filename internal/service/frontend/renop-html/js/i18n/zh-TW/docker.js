/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/* Docker / OCI 容器映像檔儲存庫瀏覽與管理。 */
export default Object.freeze({
    "docker.imagesTitle": "容器映像檔",
    "docker.imagesSubtitle": "瀏覽與管理 Docker / OCI 容器映像檔及其標籤",
    "docker.noImages": "此儲存庫中未找到任何容器映像檔。",
    "docker.searchPlaceholder": "搜尋容器映像檔...",
    "docker.tagCount": "{count} 個標籤",
    "docker.latestTag": "最新: {tag}",
    "docker.pullCommand": "拉取指令",
    "docker.tagsTitle": "標籤與 Manifest",
    "docker.tag": "標籤",
    "docker.digest": "摘要",
    "docker.size": "大小",
    "docker.created": "建立時間",
    "docker.deleteTag": "刪除標籤",
    "docker.deleteTagConfirm": "確定要永久刪除映像檔 \"{image}\" 的標籤 \"{tag}\" 嗎？",
    "docker.deleteImage": "刪除映像檔",
    "docker.deleteImageConfirm": "確定要永久刪除映像檔 \"{image}\" 及其所有關聯的標籤和 Manifest 嗎？",
    "docker.tagDeleted": "標籤已刪除。",
    "docker.imageDeleted": "映像檔已刪除。",
    "docker.inspect": "檢視 Manifest",
    "docker.copyPull": "複製拉取指令",
    "docker.copied": "已複製！",
    "docker.kickerRegistry": "Docker 倉庫",
    "docker.kickerImage": "容器映像",
    "docker.mediaType": "媒體類型",
    "docker.configDigest": "設定摘要",
    "docker.layers": "圖層",
    "docker.layerCount": "{count} 個圖層",
    "docker.manifestDetails": "Manifest 詳情",
    "docker.rawManifest": "原始 Manifest JSON",
    "docker.updated": "更新時間",
    "docker.totalImages": "{count} 個映像",
    "docker.totalTags": "{count} 個標籤",
    "docker.copyDigest": "複製摘要",
    "docker.copyDigestSuccess": "摘要已複製！",
    "docker.quickPull": "拉取",
    "docker.pushGuidance": "使用 Docker CLI 向此倉庫推送容器映像：",
    "docker.manifestLoadFailed": "載入 Manifest 詳情失敗。",
    "docker.backToImages": "返回映像列表",
    "docker.overview": "概覽",
    "docker.readme": "README",
    "docker.noReadme": "此容器映像尚未提供 README 或說明文件。",
    "docker.editReadme": "編輯 README",
    "docker.saveReadme": "儲存 README",
    "docker.readmeSaved": "README 已成功更新。",
    "docker.readmePlaceholder": "為此容器映像編寫 Markdown 格式的說明或文件……",
    "docker.publisher": "發布者",
    "docker.publishedBy": "由 {name} 發布",
    "docker.noTags": "此容器映像暫無標籤或 Manifest。"
});
