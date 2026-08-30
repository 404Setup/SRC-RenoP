/* npm repository UI, protocol help, and audit actions. */
import base from '../zh-TW/npm.js';

export default Object.freeze({
    ...base,
    "repos.formatNpmDesc": "兼容 npm、pnpm、Yarn 同 Bun 嘅 JavaScript 套件儲存庫",
    "details.npmSubtitle": "設定客戶端、安裝套件並向已保留嘅名稱發布版本",
    "npm.repositoryDescription": "發布同鏡像不可變嘅 JavaScript 套件版本，並按套件管理 L0-L4 權限。",
    "npm.noPackages": "呢個儲存庫暫時冇可用嘅 npm 套件。",
    "npm.createPackageHint": "用 npm 客戶端發布之前，必須先保留套件名稱。",
    "npm.packageCreationQueued": "npm 套件建立申請正在等待審核",
    "npm.privateRequiresScope": "私有 npm 套件必須使用 @team/package 呢類作用域名稱。",
    "npm.authenticationRequired": "請登入先管理呢個 npm 套件",
    "npm.cannotInviteSelf": "唔可以邀請自己",
    "npm.permissionDenied": "你冇權執行呢個 npm 套件操作",
    "audit.action.NPM_PACKAGE_CREATE": "建立 npm 套件",
    "audit.action.NPM_PUBLISH": "發布 npm 版本",
    "audit.action.NPM_METADATA_UPDATE": "更新 npm 套件中繼資料",
    "audit.action.NPM_VERSION_DELETE": "取消發布 npm 版本",
    "audit.action.NPM_PACKAGE_ARCHIVE": "封存 npm 套件",
    "audit.action.NPM_PACKAGE_RESTORE": "還原 npm 套件",
    "audit.action.NPM_PACKAGE_DELETE": "刪除 npm 套件",
    "audit.action.NPM_DIST_TAG": "更新 npm Dist-Tag",
    "audit.action.NPM_TEAM_ADD": "新增 npm 團隊成員",
    "audit.action.NPM_TEAM_INVITE": "傳送 npm 團隊邀請",
    "audit.action.NPM_TEAM_LEVEL": "更新 npm 團隊權限",
    "audit.action.NPM_TEAM_REMOVE": "移除 npm 團隊成員",
    "audit.action.NPM_INVITE_ACCEPT": "接受 npm 邀請",
    "audit.action.NPM_INVITE_REJECT": "拒絕 npm 邀請",
    "npm.reviewPending": "審核中"
});
