/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package audit

const (
	ActionLogin                  = "LOGIN"
	ActionLogout                 = "LOGOUT"
	ActionUpload                 = "UPLOAD"
	ActionUploadQueuedGPG        = "UPLOAD_QUEUED_GPG"
	ActionUploadQueuedReview     = "UPLOAD_QUEUED_REVIEW"
	ActionDelete                 = "DELETE"
	ActionPasswordUpdate         = "PASSWORD_UPDATE"
	ActionFIDOUpdate             = "FIDO_UPDATE"
	ActionSettingsUpdate         = "SETTINGS_UPDATE"
	ActionSessionRevoke          = "SESSION_REVOKE"
	ActionTokenGenerate          = "TOKEN_GENERATE"
	ActionTokenRevoke            = "TOKEN_REVOKE"
	ActionUserPermissionUpdate   = "USER_PERMISSION_UPDATE"
	ActionLogClear               = "LOG_CLEAR"
	ActionGPGUpdate              = "GPG_UPDATE"
	ActionProfileUpdate          = "PROFILE_UPDATE"
	ActionMessageSend            = "MESSAGE_SEND"
	ActionRepositoryMigrate      = "REPOSITORY_MIGRATE"
	ActionSuperTeamCreate        = "SUPER_TEAM_CREATE"
	ActionSuperTeamUpdate        = "SUPER_TEAM_UPDATE"
	ActionSuperTeamDelete        = "SUPER_TEAM_DELETE"
	ActionSuperTeamInvite        = "SUPER_TEAM_INVITE"
	ActionSuperTeamMemberAdd     = "SUPER_TEAM_MEMBER_ADD"
	ActionSuperTeamMemberLevel   = "SUPER_TEAM_MEMBER_LEVEL"
	ActionSuperTeamMemberRemove  = "SUPER_TEAM_MEMBER_REMOVE"
	ActionSuperTeamInvitation    = "SUPER_TEAM_INVITATION"
	ActionSuperTeamLimit         = "SUPER_TEAM_LIMIT"
	ActionPublicationQuotaUpdate = "PUBLICATION_QUOTA_UPDATE"
	ActionReviewRequest          = "REVIEW_REQUEST"
	ActionReviewDecision         = "REVIEW_DECISION"
	ActionReviewCancel           = "REVIEW_CANCEL"

	ActionCargoPublish        = "CARGO_PUBLISH"
	ActionCargoDocsUpload     = "CARGO_DOCS_UPLOAD"
	ActionCargoDocsDelete     = "CARGO_DOCS_DELETE"
	ActionCargoYank           = "CARGO_YANK"
	ActionCargoUnyank         = "CARGO_UNYANK"
	ActionCargoVersionDelete  = "CARGO_VERSION_DELETE"
	ActionCargoPackageArchive = "CARGO_PACKAGE_ARCHIVE"
	ActionCargoPackageRestore = "CARGO_PACKAGE_RESTORE"
	ActionCargoPackageDelete  = "CARGO_PACKAGE_DELETE"
	ActionCargoTeamAdd        = "CARGO_TEAM_ADD"
	ActionCargoTeamInvite     = "CARGO_TEAM_INVITE"
	ActionCargoTeamRemove     = "CARGO_TEAM_REMOVE"
	ActionCargoTeamLevel      = "CARGO_TEAM_LEVEL"
	ActionCargoInviteAccept   = "CARGO_INVITE_ACCEPT"
	ActionCargoInviteReject   = "CARGO_INVITE_REJECT"

	ActionDockerManifestPut    = "DOCKER_MANIFEST_PUT"
	ActionDockerManifestDelete = "DOCKER_MANIFEST_DELETE"
	ActionDockerBlobUpload     = "DOCKER_BLOB_UPLOAD"
	ActionDockerBlobMount      = "DOCKER_BLOB_MOUNT"
	ActionDockerBlobDelete     = "DOCKER_BLOB_DELETE"
	ActionDockerImageCreate    = "DOCKER_IMAGE_CREATE"
	ActionDockerImageUpdate    = "DOCKER_IMAGE_UPDATE"
	ActionDockerImageDelete    = "DOCKER_IMAGE_DELETE"
	ActionDockerTagDelete      = "DOCKER_TAG_DELETE"
	ActionDockerTeamAdd        = "DOCKER_TEAM_ADD"
	ActionDockerTeamInvite     = "DOCKER_TEAM_INVITE"
	ActionDockerTeamLevel      = "DOCKER_TEAM_LEVEL"
	ActionDockerTeamRemove     = "DOCKER_TEAM_REMOVE"
	ActionDockerInviteAccept   = "DOCKER_INVITE_ACCEPT"
	ActionDockerInviteReject   = "DOCKER_INVITE_REJECT"

	ActionNPMPackageCreate  = "NPM_PACKAGE_CREATE"
	ActionNPMPublish        = "NPM_PUBLISH"
	ActionNPMMetadataUpdate = "NPM_METADATA_UPDATE"
	ActionNPMVersionDelete  = "NPM_VERSION_DELETE"
	ActionNPMPackageArchive = "NPM_PACKAGE_ARCHIVE"
	ActionNPMPackageRestore = "NPM_PACKAGE_RESTORE"
	ActionNPMPackageDelete  = "NPM_PACKAGE_DELETE"
	ActionNPMDistTag        = "NPM_DIST_TAG"
	ActionNPMTeamAdd        = "NPM_TEAM_ADD"
	ActionNPMTeamInvite     = "NPM_TEAM_INVITE"
	ActionNPMTeamLevel      = "NPM_TEAM_LEVEL"
	ActionNPMTeamRemove     = "NPM_TEAM_REMOVE"
	ActionNPMInviteAccept   = "NPM_INVITE_ACCEPT"
	ActionNPMInviteReject   = "NPM_INVITE_REJECT"

	ActionMavenDomainCreate      = "MAVEN_DOMAIN_CREATE"
	ActionMavenDomainVerify      = "MAVEN_DOMAIN_VERIFY"
	ActionMavenDomainForceVerify = "MAVEN_DOMAIN_FORCE_VERIFY"
	ActionMavenDomainDelete      = "MAVEN_DOMAIN_DELETE"
	ActionMavenArtifactUpdate    = "MAVEN_ARTIFACT_UPDATE"
	ActionMavenVersionDelete     = "MAVEN_VERSION_DELETE"
	ActionMavenTeamAdd           = "MAVEN_TEAM_ADD"
	ActionMavenTeamInvite        = "MAVEN_TEAM_INVITE"
	ActionMavenTeamLevel         = "MAVEN_TEAM_LEVEL"
	ActionMavenTeamRemove        = "MAVEN_TEAM_REMOVE"
	ActionMavenTeamInvitation    = "MAVEN_TEAM_INVITATION"
)

// KnownActions returns every stable action identifier emitted by the audit service.
func KnownActions() []string {
	return []string{
		ActionLogin,
		ActionLogout,
		ActionUpload,
		ActionUploadQueuedGPG,
		ActionUploadQueuedReview,
		ActionDelete,
		ActionPasswordUpdate,
		ActionFIDOUpdate,
		ActionSettingsUpdate,
		ActionSessionRevoke,
		ActionTokenGenerate,
		ActionTokenRevoke,
		ActionUserPermissionUpdate,
		ActionLogClear,
		ActionGPGUpdate,
		ActionProfileUpdate,
		ActionMessageSend,
		ActionRepositoryMigrate,
		ActionSuperTeamCreate,
		ActionSuperTeamUpdate,
		ActionSuperTeamDelete,
		ActionSuperTeamInvite,
		ActionSuperTeamMemberAdd,
		ActionSuperTeamMemberLevel,
		ActionSuperTeamMemberRemove,
		ActionSuperTeamInvitation,
		ActionSuperTeamLimit,
		ActionPublicationQuotaUpdate,
		ActionReviewRequest,
		ActionReviewDecision,
		ActionReviewCancel,
		ActionCargoPublish,
		ActionCargoDocsUpload,
		ActionCargoDocsDelete,
		ActionCargoYank,
		ActionCargoUnyank,
		ActionCargoVersionDelete,
		ActionCargoPackageArchive,
		ActionCargoPackageRestore,
		ActionCargoPackageDelete,
		ActionCargoTeamAdd,
		ActionCargoTeamInvite,
		ActionCargoTeamRemove,
		ActionCargoTeamLevel,
		ActionCargoInviteAccept,
		ActionCargoInviteReject,
		ActionDockerManifestPut,
		ActionDockerManifestDelete,
		ActionDockerBlobUpload,
		ActionDockerBlobMount,
		ActionDockerBlobDelete,
		ActionDockerImageCreate,
		ActionDockerImageUpdate,
		ActionDockerImageDelete,
		ActionDockerTagDelete,
		ActionDockerTeamAdd,
		ActionDockerTeamInvite,
		ActionDockerTeamLevel,
		ActionDockerTeamRemove,
		ActionDockerInviteAccept,
		ActionDockerInviteReject,
		ActionNPMPackageCreate,
		ActionNPMPublish,
		ActionNPMMetadataUpdate,
		ActionNPMVersionDelete,
		ActionNPMPackageArchive,
		ActionNPMPackageRestore,
		ActionNPMPackageDelete,
		ActionNPMDistTag,
		ActionNPMTeamAdd,
		ActionNPMTeamInvite,
		ActionNPMTeamLevel,
		ActionNPMTeamRemove,
		ActionNPMInviteAccept,
		ActionNPMInviteReject,
		ActionMavenDomainCreate,
		ActionMavenDomainVerify,
		ActionMavenDomainForceVerify,
		ActionMavenDomainDelete,
		ActionMavenArtifactUpdate,
		ActionMavenVersionDelete,
		ActionMavenTeamAdd,
		ActionMavenTeamInvite,
		ActionMavenTeamLevel,
		ActionMavenTeamRemove,
		ActionMavenTeamInvitation,
	}
}
