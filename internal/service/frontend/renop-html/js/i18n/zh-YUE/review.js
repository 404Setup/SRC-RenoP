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
    "ticket.report": "舉報",
    "ticket.reportVersion": "舉報版本",
    "ticket.kind": "類型",
    "ticket.kind.feedback": "意見回饋",
    "ticket.kind.suggestion": "建議",
    "ticket.kind.report": "舉報",
    "ticket.status.unprocessed": "未處理",
    "ticket.status.in_progress": "處理中",
    "ticket.status.processed": "已處理",
    "ticket.status.closed": "已關閉",
    "ticket.status.completed": "已完成",
    "ticket.status.all": "所有狀態",
    "ticket.type.user": "用戶",
    "ticket.type.superteam": "全域團隊",
    "ticket.type.support": "支援",
    "ticket.action.claim": "接管",
    "ticket.action.force_claim": "強制接管",
    "ticket.action.release": "釋放",
    "ticket.action.escalate": "升級",
    "ticket.action.process": "標記已處理",
    "ticket.action.complete": "完成",
    "ticket.action.close": "關閉",
    "ticket.outcome": "處理結果",
    "ticket.outcome.upheld": "舉報成立",
    "ticket.outcome.dismissed": "舉報唔成立",
    "ticket.outcome.resolved": "已解決",
    "ticket.outcome.closed": "已關閉",
    "ticket.response": "畀提交者嘅回覆",
    "ticket.globalScope": "全站",
    "ticket.scope": "套件庫範圍",
    "ticket.subject": "主題",
    "ticket.body": "詳細說明",
    "ticket.created": "工單已提交。",
    "ticket.adminOnly": "等候系統管理員處理",
    "ticket.assignee": "處理人：{name}",
    "ticket.escalatedBy": "升級人：{name}",
    "ticket.escalations": "升級次數：{count}/3",
    "ticket.claimRequired": "請先接管呢張工單再處理。",
    "ticket.occupied": "呢張工單已經有處理人，請重新整理睇可用操作。",
    "ticket.escalationLimit": "呢張工單已升級三次，必須由最後接管人處理。",
    "ticket.confirm.force_claim": "要從目前處理人接管呢張工單？",
    "ticket.confirm.release": "要釋放呢張工單，畀其他有權限嘅處理人領取？",
    "ticket.confirm.escalate": "要釋放工單並升級畀另一位系統管理員？升級三次之後，最後接管人必須處理。",
    "ticket.responsePrivacy": "提交者會睇到呢個回覆，唔好包處理人身分或者私隱資訊。舉報成立嗰陣，請先執行對應處置再確認結果。",
    "ticket.reportPrivacy": "被舉報方睇唔到你嘅身分或者舉報內容，處理結果會透過訊息中心通知。",
    "ticket.createHint": "選擇對應儲存庫聯絡版主，或者選擇全站聯絡系統管理員。"
});
