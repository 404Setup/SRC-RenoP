/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import base from '../zh-HK/review.js';

export default Object.freeze({
    ...base,
    "audit.action.REVIEW_REQUEST": "提交審核",
    "audit.action.REVIEW_DECISION": "處理審核",
    "audit.action.REVIEW_CANCEL": "取消審核申請",
    "ticket.open": "開啟工單",
    "ticket.create": "建立工單",
    "ticket.report": "檢舉",
    "ticket.reportVersion": "檢舉版本",
    "ticket.kind": "類型",
    "ticket.kind.feedback": "意見回饋",
    "ticket.kind.suggestion": "建議",
    "ticket.kind.report": "檢舉",
    "ticket.status.unprocessed": "未處理",
    "ticket.status.in_progress": "處理中",
    "ticket.status.processed": "已處理",
    "ticket.status.closed": "已關閉",
    "ticket.status.completed": "已完成",
    "ticket.status.all": "所有狀態",
    "ticket.type.user": "使用者",
    "ticket.type.superteam": "全域團隊",
    "ticket.type.support": "支援",
    "ticket.action.claim": "接管",
    "ticket.action.force_claim": "強制接管",
    "ticket.action.release": "釋出",
    "ticket.action.escalate": "升級",
    "ticket.action.process": "標記已處理",
    "ticket.action.complete": "完成",
    "ticket.action.close": "關閉",
    "ticket.outcome": "處理結果",
    "ticket.outcome.upheld": "檢舉成立",
    "ticket.outcome.dismissed": "檢舉不成立",
    "ticket.outcome.resolved": "已解決",
    "ticket.outcome.closed": "已關閉",
    "ticket.response": "給提交者的回覆",
    "ticket.globalScope": "全站",
    "ticket.scope": "儲存庫範圍",
    "ticket.subject": "主旨",
    "ticket.body": "詳細說明",
    "ticket.created": "工單已提交。",
    "ticket.adminOnly": "等待系統管理員處理",
    "ticket.assignee": "處理人：{name}",
    "ticket.escalatedBy": "升級人：{name}",
    "ticket.escalations": "升級次數：{count}/3",
    "ticket.claimRequired": "請先接管此工單再處理。",
    "ticket.occupied": "此工單已有處理人，請重新整理以查看可用操作。",
    "ticket.escalationLimit": "此工單已升級三次，必須由最終接管人處理。",
    "ticket.confirm.force_claim": "從目前處理人處接管此工單？",
    "ticket.confirm.release": "釋出此工單，供其他有權限的處理人領取？",
    "ticket.confirm.escalate": "釋出工單並升級給另一位系統管理員？升級三次後，最終接管人必須處理。",
    "ticket.responsePrivacy": "提交者將看到此回覆。請勿包含處理人員身分或帳號隱私。檢舉成立時，請先透過資源或帳號管理功能執行處罰，再確認結果。",
    "ticket.reportPrivacy": "被檢舉方無法看到你的身分或檢舉內容。最終結果會透過站內訊息通知，並在啟用郵件時寄送郵件。",
    "ticket.createHint": "選擇儲存庫以聯絡其版主，或選擇全站以聯絡系統管理員。"
});
