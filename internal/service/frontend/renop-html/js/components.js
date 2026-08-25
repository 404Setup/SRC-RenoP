/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/**
 * Barrel re-exports for renop-html UI component factories and custom elements.
 * Symbol docs live on the defining modules under `./components/`.
 */

export { ICONS, RenopIcon, createIcon } from './components/icon.js';
export { RenopBadge, createBadge } from './components/badge.js';
export { RenopCallout, createCallout } from './components/callout.js';
export { RenopSkeleton, createSkeleton } from './components/skeleton.js';
export { RenopStatCard, createStatCard } from './components/stat-card.js';
export {createButton, runButtonAction} from './components/button.js';
export { RenopSection } from './components/section.js';
export { RenopSubHeader, createSubHeader } from './components/sub-header.js';
export { RenopCard, createIndexCard } from './components/card.js';
export { RenopDialog } from './components/dialog.js';
export { RenopUserAvatar, createUserAvatar } from './components/user-avatar.js';
export { RenopUserIdentity, createUserIdentity } from './components/user-identity.js';
export { RenopMetaChip, createMetaChip } from './components/meta-chip.js';
export { RenopMirrorCard, createMirrorCard } from './components/mirror-card.js';
export { RenopRoleChip, createRoleChip } from './components/role-chip.js';
export { RenopMetaGrid, createMetaGrid } from './components/meta-grid.js';
export { RenopToggle, createToggle } from './components/toggle.js';
export { RenopFieldRow, createFieldRow, createToggleRow } from './components/field-row.js';
export { RenopDropzone, createDropzone } from './components/dropzone.js';
export { RenopRolesGroup, createRolesGroup } from './components/roles-group.js';
export { RenopEmptyState, createEmptyState } from './components/empty-state.js';
export { RenopAlert, createAlert } from './components/alert.js';
export { RenopCodeBadge, createCodeBadge } from './components/code-badge.js';
export { RenopRepoRow, createRepoRow } from './components/repo-row.js';
export { RenopFileCard, createFileCard } from './components/file-card.js';
export { RenopUploadEntry, createUploadEntry } from './components/upload-entry.js';
export { RenopLangCard, createLangCard } from './components/lang-card.js';
export { getFileTypeInfo, getFileTypeLabel, RenopFileItem, createFileItem, getFileTypeCategory, getCategoryIconName, FILE_TYPE_CATALOG, SPECIAL_FILE_TYPES, EXTENSION_FILE_TYPES } from './components/file-item.js';
export { createUsersSkeletonRow, RenopUserRow, createUserRow } from './components/user-row.js';
export { RenopChartBar, createChartBar } from './components/chart-bar.js';
export { RenopTab, createTab, createBreadcrumbSep, createBreadcrumbLink } from './components/tab.js';
