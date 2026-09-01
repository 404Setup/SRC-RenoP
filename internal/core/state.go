/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package core defines shared application state and domain models.
package core

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	atomic2 "sync/atomic/v2"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/llxisdsh/pb"

	"renop/internal/config"
	"renop/internal/service/index"
)

type AuthCacheEntry struct {
	User           *config.User
	CredentialKind string
	AuthScheme     string
	APITokenID     string
	Scopes         []string
	Targets        map[string][]string
	ExpiredAt      int64
	Invalid        bool
}

var ErrDatabaseUnavailable = errors.New("database unavailable")

type CachedFile struct {
	Bytes        []byte
	Etag         *string
	LastModified *string
	ContentType  string
}

type StatusSnapshot struct {
	Timestamp   int64  `json:"timestamp"`
	UsedMemory  uint64 `json:"used_memory"`
	VssMemory   uint64 `json:"vss_memory"`
	UsedThreads uint64 `json:"used_threads"`
	OpenFiles   uint64 `json:"open_files"`
}

type StateDB interface {
	GetTokenByName(name string) (*AccessToken, error)
	GetTokenByEmail(email string) (*AccessToken, error)
	GetTokenBySecret(secret string) (*AccessToken, error)
	GetAllTokens() ([]*AccessToken, error)
	UpdateToken(name string, updateFn func(*AccessToken)) error
	ListAPITokens(username string) ([]*APIToken, error)
	CreateAPIToken(username string, token *APIToken, secretHash string) error
	DeleteAPIToken(username, tokenID string) error
	GetAPITokenByHash(secretHash, username string) (*APITokenCredential, error)
	CountAPITokens(username string) (int, error)
	CountAPITokensByUsername() (map[string]int, error)
	SearchTokenNames(prefix string, limit int, now int64) ([]string, error)
	CountTokens() (uint64, error)
	SaveToken(token *AccessToken) error
	CreateToken(token *AccessToken, nickname string, changedAt int64) error
	DeleteToken(name string) error
	RenameToken(oldName, newName string, token *AccessToken) error
	GetUserProfile(username string) (*UserProfile, error)
	GetUserProfileByID(userID string) (*UserProfile, error)
	GetUserProfiles(usernames []string) (map[string]*UserProfile, error)
	GetUserAvatar(username string) (*UserAvatar, error)
	PutUserAvatar(username string, avatar *UserAvatar) error
	DeleteUserAvatar(username string) error
	ListUserPackageMemberships(userID, format string) ([]*UserPackageMembership, error)
	UpdateUserProfile(oldUsername, newUsername, nickname string, token *AccessToken, changedAt int64, changes AccountTokenChanges) (*UserProfile, error)
	GetSession(sessionToken string) (*Session, error)
	SaveSession(session *Session, sessionToken string) error
	UpdateSessionLastActive(sessionToken string, lastActive int64) error
	DeleteSession(sessionToken string) error
	DeleteSessionsByUsername(username string) error
	ListUserSessions(username, currentSessionToken string) ([]SessionDto, error)
	DeleteExpiredSessions(minActiveTimestamp int64) error
	DeleteUserSessionByPublicID(username, publicID, currentSessionToken string) (token string, revoked bool, wasCurrent bool, err error)
	DeleteOtherUserSessions(username, keepSessionToken string) (tokens []string, err error)
	GetActiveSessions(minActiveTimestamp int64) ([]SessionDBDto, error)
	UpdateSessionsUsername(oldUsername, newUsername string) error
	ListFidoDevices(username string) ([]*FidoDevice, error)
	GetFidoDeviceByCredentialID(credentialID []byte) (*FidoDevice, error)
	SaveFidoDevice(device *FidoDevice) error
	UpdateFidoSignCount(credentialID []byte, signCount uint32) error
	UpdateFidoDeviceState(credentialID []byte, signCount uint32, backupState bool, backupEligible bool) error
	DeleteFidoDevice(username, deviceID string) error
	DeleteFidoDevicesByUsername(username string) error
	GetAccountSecurity(username string) (*AccountSecurity, error)
	PasswordLoginEnabled(username string) (bool, error)
	UpdateAccountEmail(username, email string, updatedAt int64) (*AccountSecurity, error)
	SetPasswordLoginEnabled(username string, enabled bool, updatedAt int64) (*AccountSecurity, error)
	SetAccountPassword(username, passwordHash string, updatedAt int64) error
	ReplaceRecoveryCodes(username string, codes []RecoveryCodeHash) error
	GetRecoveryCodes(identifier string, selectorHashes []string) (string, []RecoveryCodeRecord, error)
	ResetPasswordWithRecoveryCodes(identifier string, selectorHashes []string, passwordHash string, updatedAt int64) (string, error)
	GetGitHubIdentity(username string) (*GitHubIdentity, error)
	GetGitHubIdentityByProviderID(githubUserID int64) (*GitHubIdentity, error)
	StoreGitHubIdentity(userID string, githubUserID int64, githubLogin string, principals []GitHubPrincipal, authorizedAt int64) error
	DeleteGitHubIdentity(username string) error
	HasRecentGitHubPrincipal(username, login string, authorizedAfter int64) (bool, error)
	FindGPGPublicKeys(identifier string) ([]*GPGPublicKey, error)
	GetGPGPublicKey(fingerprint string) (*GPGPublicKey, error)
	ListUserGPGKeys(username string) ([]*UserGPGKey, error)
	RegisterUserGPGKey(username, requestedID string, key *GPGPublicKey, aliases []string) error
	RefreshGPGPublicKey(key *GPGPublicKey, aliases []string) error
	DeleteUserGPGKey(username, fingerprint string) error
	SaveGPGSignature(signature *GPGSignature) error
	GetGPGSignature(artifactKey string) (*GPGSignature, error)
	GetGPGSignatures(artifactKeys []string) ([]*GPGSignature, error)
	DeleteGPGSignature(artifactKey string) error
	DeleteGPGSignaturesByPrefix(repository, artifactPathPrefix string) error
	DeleteGPGSignaturesByRepository(repository string) error
	SaveGPGRelease(release *GPGRelease) error
	GetActiveGPGRelease(activeKey string) (*GPGRelease, error)
	ClaimNextGPGRelease(optionalReadyBefore int64) (*GPGRelease, error)
	ListGPGReleases(username string, limit, offset int) ([]*GPGRelease, int, error)
	ListPendingGPGReleases() ([]*GPGRelease, error)
	CountPendingGPGReleases(username string) (int, int, error)
	ResetValidatingGPGReleases() error
	SaveAuditLog(entry *AuditLogEntry) error
	GetAuditLogs(username string, limit, offset int) ([]*AuditLogEntry, int, error)
	DeleteAuditLogsByUsername(username string) error
	CleanExpiredAuditLogs(retentionDays int, maxRows int) error
	SaveMessages(messages []*UserMessage) error
	SaveMessageIfAbsent(message *UserMessage) (bool, error)
	ListMessages(username string, limit int, beforeCreatedAt int64, beforeID string, now int64) ([]*UserMessage, error)
	CountUnreadMessages(username string, now int64) (int, error)
	GetUserMessage(id, username string, now int64) (*UserMessage, error)
	MarkMessageRead(id, username string, readAt int64) (bool, error)
	MarkAllMessagesRead(username string, readAt int64) (int64, error)
	TransitionMessageAction(id, username, expectedStatus, newStatus string, actedAt int64) (bool, error)
	DeleteUserMessage(id, username string) (bool, error)
	DeleteUserMessages(username string) (int64, error)
	DeleteMessagesByDedupeKey(dedupeKey string) (int64, error)
	GetPublicationQuotaStatus(subject PublicationQuotaSubject, defaults PublicationQuotaLimits, now int64) (*PublicationQuotaStatus, error)
	SetPublicationQuotaOverride(subject PublicationQuotaSubject, override PublicationQuotaOverride, updatedAt int64) error
	ReservePublicationQuota(subject PublicationQuotaSubject, defaults PublicationQuotaLimits, delta PublicationQuotaDelta, now, expiresAt int64) (*PublicationQuotaReservation, error)
	CommitPublicationQuotaReservation(id string, committedAt int64) error
	ReleasePublicationQuotaReservation(id string) error
	CleanExpiredPublicationQuotaReservations(now int64) error
	GetSuperTeamLimitStatus(username string, globalCreateLimit, globalJoinLimit int) (*SuperTeamLimitStatus, error)
	SetSuperTeamLimitOverride(username string, createLimit, joinLimit *int, updatedAt int64) error
	CreateSuperTeam(team *SuperTeam, owner string, globalCreateLimit, globalJoinLimit int) error
	ListSuperTeams(username string, administrator bool, limit, offset int) ([]*SuperTeam, int, error)
	ListManageableSuperTeams(username string, minimumRole, limit, offset int) ([]*SuperTeam, int, error)
	GetSuperTeamRole(prefix, username string) (int, error)
	GetSuperTeamDetails(prefix, username string, administrator bool) (*SuperTeamDetails, error)
	GetPublicSuperTeamDetails(prefix, username string, administrator bool) (*SuperTeamDetails, error)
	ListSuperTeamReviewerNames(prefix string) ([]string, error)
	UpdateSuperTeam(prefix, actor, name, description string, administrator bool, updatedAt int64) error
	DeleteSuperTeam(prefix, actor string, administrator bool, actedAt int64) error
	CreateSuperTeamInvitations(invitations []*SuperTeamInvitation, messages []*UserMessage) error
	ForceAddSuperTeamMembers(prefix, actor string, usernames []string, level, globalCreateLimit, globalJoinLimit int, actedAt int64) error
	RespondSuperTeamInvitation(id, recipient string, accept bool, globalJoinLimit int, actedAt int64) error
	SetSuperTeamMemberLevel(prefix, actor, target string, level int, administrator bool) error
	RemoveSuperTeamMember(prefix, actor, target string, administrator bool, actedAt int64) error
	CleanExpiredSuperTeamInvitations(now int64) error
	CreateSuperTeamTransferReview(request SuperTeamTransferRequest, actor string, administrator bool, createdAt int64) (*ReviewTask, error)
	CreateOrUpdatePublicationReview(request PublicationReviewRequest) (*PublicationReviewResult, error)
	ListReviewTasks(options ReviewTaskListOptions) ([]*ReviewTask, int, error)
	GetReviewTask(id string) (*ReviewTask, error)
	ListReviewTaskFiles(id string) ([]*ReviewFile, error)
	GetReviewTaskPayload(id string) ([]byte, error)
	ListPendingPublicationReviewFiles() ([]*ReviewFile, error)
	IsPublicationReviewPathPending(repository, path string) (bool, error)
	HasPendingPublicationReviews(repository string) (bool, error)
	ListPublicationReviews(repository, resourceType, resourceName string) ([]*ReviewTask, error)
	AdvancePackageCreationReview(id, actor string, advancedAt int64) (*ReviewTask, error)
	DecideReviewTask(id, actor, decision, reason string, decidedAt int64) (*ReviewTask, error)
	CancelReviewTask(id, actor string, cancelledAt int64) (*ReviewTask, error)
	CreateMavenDomain(domain *MavenDomain, owner string) error
	ListMavenDomains(username string, includeAll bool) ([]*MavenDomain, error)
	ListManagedMavenDomains(options MavenDomainListOptions) ([]*MavenDomain, int, error)
	ListMavenRepositoryDomains(repository, username string) ([]*MavenDomain, error)
	SearchMavenRepositoryDomains(repository, query string, limit int) ([]*MavenDomain, int, error)
	GetMavenDomainDetails(domain, username string) (*MavenDomainDetails, error)
	ReserveMavenVerificationAttempt(domain, actor string, administrator bool, checkedAt, minimumPrevious int64) error
	MarkMavenDomainVerified(domain, code string, verifiedAt int64) error
	DeleteMavenDomain(domain, actor string, administrator bool, actedAt int64) error
	HasMavenMembership(username string) (bool, error)
	RecordMavenPublication(artifact *MavenArtifact, version *MavenVersion) error
	RecordMavenMirrorPublication(artifact *MavenArtifact, version *MavenVersion) error
	MavenArtifactExists(repository, groupID, artifactID string) (bool, error)
	ListMavenArtifacts(repository, domain, query string, limit, offset int) ([]*MavenArtifact, int, error)
	ListMavenDomainArtifacts(repositories []string, domain string, limit, offset int) ([]*MavenArtifact, int, error)
	GetMavenArtifactDetails(repository, groupID, artifactID string) (*MavenArtifactDetails, error)
	GetMavenArtifactTeamAccess(repository, groupID, artifactID, username string) (string, bool, int, error)
	UpdateMavenArtifactDescription(repository, groupID, artifactID, description string) error
	UpdateMavenArtifactReadme(repository, groupID, artifactID, readme string) error
	DeleteMavenVersionMetadata(repository, groupID, artifactID, version string) error
	DeleteMavenRepository(repository string) error
	EnsureImportedMavenDomain(domain *MavenDomain) error
	EnsureMirroredMavenDomain(domain string, createdAt int64) error
	IsMavenRepositoryUpgraded(repository string) (bool, error)
	MarkMavenRepositoryUpgraded(repository string, completedAt int64) error
	CreateMavenInvitations(invitations []*MavenInvitation, messages []*UserMessage) error
	ForceAddMavenMembers(domain, actor string, usernames []string, level int) error
	RespondMavenInvitation(id, recipient string, accept bool, actedAt int64) error
	SetMavenMemberLevel(domain, actor, username string, level int) error
	RemoveMavenMember(domain, actor, username string) error
	GetCargoPackage(repository, normalizedName string) (*CargoPackage, error)
	CargoHasPublishedVersions(repository, normalizedName string) (bool, error)
	GetCargoPackageDetails(repository, normalizedName, username string) (*CargoPackageDetails, error)
	ListCargoPackages(repository, username string, administrator bool) ([]*CargoPackage, error)
	SearchCargoPackages(repository, query string, limit, offset int) ([]*CargoPackage, int, error)
	HasCargoMembership(repository, username string) (bool, error)
	RecordCargoPublication(pkg *CargoPackage, version *CargoVersion, username string) error
	RollbackCargoPublicationReview(repository, normalizedName, version string, previous *CargoPackage) error
	RecordCargoMirrorPublication(pkg *CargoPackage, version *CargoVersion) error
	SetCargoVersionYanked(repository, normalizedName, version string, yanked, administrator bool) error
	DeleteCargoVersion(repository, normalizedName, version string) error
	SetCargoPackageArchived(repository, normalizedName string, archived, administrator bool) error
	DeleteCargoPackage(repository, normalizedName string, actedAt int64) error
	DeleteCargoRepository(repository string, actedAt int64) error
	CreateCargoInvitations(invitations []*CargoInvitation, messages []*UserMessage) error
	RespondCargoInvitation(id, recipient, repository string, accept bool, actedAt int64) error
	SetCargoMemberLevel(repository, normalizedName, actor, username string, level int) error
	RemoveCargoMember(repository, normalizedName, actor, username string) error
	RemoveCargoMembers(repository, normalizedName, actor string, usernames []string) error
	ForceAddCargoMembers(repository, normalizedName, crateName, actor string, usernames []string, level int) error
	CreateNPMPackage(repository, packageName, owner string, private bool, createdAt int64) (*NPMPackage, error)
	CreateNPMPackageForTeam(repository, packageName, owner, superTeamPrefix string, private bool, createdAt int64) (*NPMPackage, error)
	GetNPMPackage(repository, packageName string) (*NPMPackage, error)
	GetNPMPackageAccess(repository, packageName, username string) (exists, private, publishEnabled, member bool, level int, err error)
	GetNPMPackageDetails(repository, packageName, username string) (*NPMPackageDetails, error)
	ListNPMPackages(repository, username string, administrator bool, limit, offset int) ([]*NPMPackage, int, error)
	SearchNPMPackages(repository, query, username string, administrator bool, limit, offset int) ([]*NPMPackage, int, error)
	HasNPMMembership(repository, username string) (bool, error)
	RecordNPMPublication(pkg *NPMPackage, version *NPMVersion, tags map[string]string, username string) error
	RollbackNPMPublicationReview(repository, packageName, version string, previous *NPMPackage,
		previousTags map[string]string) error
	RecordNPMMirrorPublication(pkg *NPMPackage, versions []*NPMVersion, tags map[string]string) error
	UpdateNPMTarballSize(repository, tarballPath string, size int64) error
	SetNPMDistTag(repository, packageName, tag, version, actor string, expectedRevision int64) error
	DeleteNPMDistTag(repository, packageName, tag, actor string, expectedRevision int64) error
	SetNPMVersionDeprecated(repository, packageName, version, deprecated, actor string, expectedRevision int64) error
	UpdateNPMPackument(repository, packageName, actor string, expectedRevision int64, deprecations, tags map[string]string) error
	UnpublishNPMVersion(repository, packageName, version, actor string, expectedRevision int64) (string, error)
	UpdateNPMPackageDescription(repository, packageName, description, actor string) error
	SetNPMPackagePrivate(repository, packageName, actor string, private bool) error
	SetNPMPackageArchived(repository, packageName, actor string, archived bool) error
	DeleteNPMPackage(repository, packageName, actor string, expectedRevision int64) ([]string, error)
	DeleteNPMRepository(repository string) error
	ListNPMMembers(repository, packageName string) ([]*NPMMember, error)
	CreateNPMInvitations(invitations []*NPMInvitation, messages []*UserMessage) error
	RespondNPMInvitation(id, recipient, repository string, accept bool, actedAt int64) error
	ForceAddNPMMembers(repository, packageName, actor string, usernames []string, level int) error
	SetNPMMemberLevel(repository, packageName, actor, username string, level int) error
	RemoveNPMMember(repository, packageName, actor, username string) error
	RemoveNPMMembers(repository, packageName, actor string, usernames []string) error
	GetDockerImage(repository, imageName string) (*DockerRepositoryImage, error)
	CreateDockerImage(repository, imageName, owner string, private bool, createdAt int64) (*DockerRepositoryImage, error)
	CreateDockerImageForTeam(repository, imageName, owner, superTeamPrefix string, private bool, createdAt int64) (*DockerRepositoryImage, error)
	GetDockerImageAccess(repository, imageName, username string) (exists, private, pushEnabled, member bool, level int, err error)
	DockerImageMemberLevels(repository, username string, imageNames []string) (map[string]int, error)
	UpdateDockerImageDescription(repository, imageName, description string) error
	ListDockerImages(repository, last string, limit int) ([]*DockerRepositoryImage, error)
	SearchDockerImages(repository, query string, limit, offset int) ([]*DockerRepositoryImage, int, error)
	GetDockerImageDetails(repository, imageName string, username ...string) (*DockerImageDetails, error)
	GetDockerTag(repository, imageName, tag string) (*DockerTag, error)
	ListDockerTags(repository, imageName, last string, limit int) ([]*DockerTag, error)
	GetDockerManifest(repository, imageName, digest string) (*DockerManifest, error)
	PutDockerManifest(manifest *DockerManifest, tag string, username string) error
	ApproveDockerPublicationReview(id, reviewer string, manifest *DockerManifest, tag string,
		decidedAt int64) (*ReviewTask, error)
	ApproveDockerImageCreationReview(id, reviewer, repository, imageName, superTeamPrefix string,
		private bool, createdAt, decidedAt int64) (*ReviewTask, error)
	ApproveNPMPackageCreationReview(id, reviewer, repository, packageName, superTeamPrefix string,
		private bool, createdAt, decidedAt int64) (*ReviewTask, error)
	CacheDockerManifest(manifest *DockerManifest, tag string) error
	DeleteDockerTag(repository, imageName, tag string) error
	DeleteDockerManifest(repository, imageName, digest string) error
	DeleteDockerImage(repository, imageName string) error
	DeleteDockerRepository(repository string) error
	RecordDockerBlob(repository, digest string, size int64) error
	RecordDockerImageBlob(repository, imageName, digest string) error
	HasDockerBlob(repository, digest string) (bool, int64, error)
	DockerImageReferencesBlob(repository, imageName, digest string) (bool, error)
	DeleteDockerBlob(repository, digest string) error
	GetDockerRepositoryStats(repository string) (totalImages int64, totalTags int64, totalSize int64, err error)
	IncrementDockerPullCount(repository, imageName string) error
	BatchIncrementDockerPullCount(repository, imageName string, delta int64) error
	BatchIncrementDownloadStatistics(events []*DownloadStatisticDelta) error
	ResetDownloadStatistics(repository string) error
	QueryDownloadStatistics(query DownloadStatisticsQuery) (*DownloadStatisticsPage, error)
	HasDockerMembership(repository, username string) (bool, error)
	GetDockerMemberLevel(repository, imageName, username string) (int, error)
	ListDockerMembers(repository, imageName string) ([]*DockerMember, error)
	CreateDockerInvitations(invitations []*DockerInvitation, messages []*UserMessage) error
	RespondDockerInvitation(id, recipient, repository string, accept bool, actedAt int64) error
	SetDockerMemberLevel(repository, imageName, actor, username string, level int) error
	RemoveDockerMember(repository, imageName, actor, username string) error
	RemoveDockerMembers(repository, imageName, actor string, usernames []string) error
	ForceAddDockerMembers(repository, imageName, actor string, usernames []string, level int) error
}

type AppStateInner struct {
	Config                      *atomic2.Value[*config.Config]
	ConfigWriteLock             sync.Mutex
	TokensCount                 atomic.Uint64
	TokenWriteLock              sync.Mutex
	StatusSnapshots             atomic.Pointer[[]StatusSnapshot]
	ActiveRequests              atomic.Uint64
	FailuresCount               atomic.Uint64
	AuthCache                   pb.MapOf[string, AuthCacheEntry]
	AuthCacheEntries            atomic.Uint64
	AuthCacheWriteLock          sync.Mutex
	Sessions                    pb.MapOf[string, *Session]
	AuditLogChan                chan *AuditLogEntry
	DB                          any
	DockerSecret                []byte
	DownloadStatisticsCounter   DownloadStatisticsCounter
	DownloadStatisticsCounterMu sync.Mutex
	ExternalAuthStates          *TransientAuthStateStore

	FileIndex              *index.FileIndex
	IndexWatcher           *fsnotify.Watcher
	IndexWatcherMutex      sync.Mutex
	StartTime              int64
	FileCache              *FileByteCache
	MetadataCache          pb.MapOf[string, *config.Metadata]
	MetadataCacheEntries   atomic.Uint64
	MetadataCacheWriteLock sync.Mutex
	InFlightDownloads      *InFlightManager
	GPGKeyFetches          *InFlightManager
	GPGUserKeyUpdates      *InFlightManager
	GPGReleaseWake         chan struct{}
	GPGReleaseWorkerActive atomic.Bool
	AnomalyFailures        *AnomalyFailureStore
	ProxyClientSemaphore   chan struct{}
}

type AppState struct {
	Inner *AppStateInner
}

func NewAppState() *AppState {
	return &AppState{
		Inner: &AppStateInner{
			Config:               &atomic2.Value[*config.Config]{},
			ProxyClientSemaphore: make(chan struct{}, 256),
			StartTime:            time.Now().UnixMilli(),
			InFlightDownloads:    NewInFlightManager(),
			GPGKeyFetches:        NewInFlightManager(),
			GPGUserKeyUpdates:    NewInFlightManager(),
			GPGReleaseWake:       make(chan struct{}, 1),
			AnomalyFailures:      NewAnomalyFailureStore(),
			AuditLogChan:         make(chan *AuditLogEntry, 500),
			ExternalAuthStates:   NewTransientAuthStateStore(),
		},
	}
}

func (state *AppState) GetDB() StateDB {
	if state == nil || state.Inner == nil || state.Inner.DB == nil {
		return nil
	}
	if sdb, ok := state.Inner.DB.(StateDB); ok {
		return sdb
	}
	return nil
}

func (state *AppState) GetDockerSecret() []byte {
	if state == nil || state.Inner == nil {
		return []byte("renop-docker-token-secret-fallback")
	}
	if len(state.Inner.DockerSecret) == 0 {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			state.Inner.DockerSecret = []byte("renop-docker-token-secret-fallback")
		} else {
			state.Inner.DockerSecret = secret
		}
	}
	return state.Inner.DockerSecret
}

func (state *AppState) GetTokenByName(name string) *AccessToken {
	if state == nil || state.Inner == nil || name == "" {
		return nil
	}
	db := state.GetDB()
	if db == nil {
		return nil
	}
	tok, err := db.GetTokenByName(name)
	if err != nil {
		return nil
	}
	return tok
}

func (state *AppState) GetTokenBySecret(secret string) *AccessToken {
	if state == nil || state.Inner == nil || secret == "" {
		return nil
	}
	db := state.GetDB()
	if db == nil {
		return nil
	}
	tok, err := db.GetTokenBySecret(secret)
	if err != nil {
		return nil
	}
	return tok
}

func (state *AppState) GetAllTokens() []*AccessToken {
	if state == nil || state.Inner == nil {
		return []*AccessToken{}
	}
	db := state.GetDB()
	if db == nil {
		return []*AccessToken{}
	}
	toks, err := db.GetAllTokens()
	if err != nil || toks == nil {
		return []*AccessToken{}
	}
	return toks
}

func (state *AppState) GetSession(sessionToken string) *Session {
	if state == nil || state.Inner == nil || sessionToken == "" {
		return nil
	}
	if sess, ok := state.Inner.Sessions.Load(sessionToken); ok && sess != nil {
		return sess
	}
	db := state.GetDB()
	if db == nil {
		return nil
	}
	sess, err := db.GetSession(sessionToken)
	if err != nil || sess == nil {
		return nil
	}
	return sess
}

func (state *AppState) SaveSession(session *Session, sessionToken string) error {
	if state == nil || state.Inner == nil || session == nil || sessionToken == "" {
		return nil
	}
	if db := state.GetDB(); db != nil {
		if err := db.SaveSession(session, sessionToken); err != nil {
			return err
		}
	}
	state.Inner.Sessions.Store(sessionToken, session)
	return nil
}

// RevokeSession removes a browser session by its secret token and invalidates related auth cache.
func (state *AppState) RevokeSession(sessionToken string) (bool, error) {
	if state == nil || state.Inner == nil || sessionToken == "" {
		return false, nil
	}
	if db := state.GetDB(); db != nil {
		if err := db.DeleteSession(sessionToken); err != nil {
			return false, err
		}
	}
	state.DeleteAuthCache("Session " + sessionToken)
	state.Inner.Sessions.Delete(sessionToken)
	return true, nil
}

// sessionToDto maps an in-memory Session (+ map key) to a public SessionDto.
// Session secret (map key) is never included.
func sessionToDto(secretToken string, session *Session, currentSessionToken string) SessionDto {
	lastActive := session.LastActive.Load()
	return SessionDto{
		PublicID:   session.PublicID,
		Username:   session.Username,
		IP:         session.IP,
		UserAgent:  session.UserAgent,
		CreatedAt:  session.CreatedAt,
		LastActive: lastActive,
		ExpiresAt:  lastActive + SessionIdleTimeoutMillis,
		Current:    secretToken != "" && secretToken == currentSessionToken,
	}
}

// ListUserSessions returns browser sessions for username (Basic/Bearer are not sessions).
// currentSessionToken is the secret token of the request's session, if any.
func (state *AppState) ListUserSessions(username, currentSessionToken string) []SessionDto {
	if state == nil || state.Inner == nil || username == "" {
		return []SessionDto{}
	}
	if db := state.GetDB(); db != nil {
		sessions, err := db.ListUserSessions(username, currentSessionToken)
		if err != nil || sessions == nil {
			return []SessionDto{}
		}
		return sessions
	}
	var sessions []SessionDto
	state.Inner.Sessions.Range(func(key string, value *Session) bool {
		if value != nil && value.Username == username {
			sessions = append(sessions, sessionToDto(key, value, currentSessionToken))
		}
		return true
	})
	if sessions == nil {
		return []SessionDto{}
	}
	return sessions
}

// RevokeUserSessionByPublicID removes one session owned by username, identified by public_id.
// Returns (revoked, wasCurrent) where wasCurrent means the revoked secret matched currentSessionToken.
func (state *AppState) RevokeUserSessionByPublicID(username, publicID, currentSessionToken string) (revoked bool, wasCurrent bool, err error) {
	if state == nil || state.Inner == nil || username == "" || publicID == "" {
		return false, false, nil
	}
	if db := state.GetDB(); db != nil {
		token, revoked, wasCurrent, err := db.DeleteUserSessionByPublicID(username, publicID, currentSessionToken)
		if err != nil {
			return false, false, err
		}
		if revoked && token != "" {
			state.DeleteAuthCache("Session " + token)
			state.Inner.Sessions.Delete(token)
		}
		return revoked, wasCurrent, nil
	}
	var toRemove string
	state.Inner.Sessions.Range(func(key string, value *Session) bool {
		if value != nil && value.Username == username && value.PublicID == publicID {
			toRemove = key
			return false
		}
		return true
	})
	if toRemove == "" {
		return false, false, nil
	}
	wasCurrent = currentSessionToken != "" && toRemove == currentSessionToken
	revoked, err = state.RevokeSession(toRemove)
	return revoked, wasCurrent, err
}

// RevokeOtherUserSessions removes every session for username except the one matching keepSessionToken.
// If keepSessionToken is empty, all sessions for the user are removed.
func (state *AppState) RevokeOtherUserSessions(username, keepSessionToken string) (int, error) {
	if state == nil || state.Inner == nil || username == "" {
		return 0, nil
	}
	if db := state.GetDB(); db != nil {
		deletedTokens, err := db.DeleteOtherUserSessions(username, keepSessionToken)
		if err != nil {
			return 0, err
		}
		toRemove := append([]string(nil), deletedTokens...)
		seen := make(map[string]struct{}, len(deletedTokens))
		for _, token := range deletedTokens {
			seen[token] = struct{}{}
		}
		state.Inner.Sessions.Range(func(token string, session *Session) bool {
			if session != nil && strings.EqualFold(session.Username, username) && token != keepSessionToken {
				if _, exists := seen[token]; !exists {
					toRemove = append(toRemove, token)
				}
			}
			return true
		})
		for _, t := range toRemove {
			state.DeleteAuthCache("Session " + t)
			state.Inner.Sessions.Delete(t)
		}
		return len(deletedTokens), nil
	}
	var toRemove []string
	state.Inner.Sessions.Range(func(key string, value *Session) bool {
		if value != nil && value.Username == username && key != keepSessionToken {
			toRemove = append(toRemove, key)
		}
		return true
	})
	count := 0
	for _, token := range toRemove {
		if revoked, err := state.RevokeSession(token); err != nil {
			return count, err
		} else if revoked {
			count++
		}
	}
	return count, nil
}

// RevokeAllUserSessions removes every browser session for username.
func (state *AppState) RevokeAllUserSessions(username string) (int, error) {
	return state.RevokeOtherUserSessions(username, "")
}

func (state *AppState) ListFidoDevices(username string) []*FidoDevice {
	if state == nil || state.Inner == nil || username == "" {
		return []*FidoDevice{}
	}
	lowerName := strings.ToLower(username)
	db := state.GetDB()
	if db == nil {
		return []*FidoDevice{}
	}
	devs, err := db.ListFidoDevices(lowerName)
	if err != nil || devs == nil {
		return []*FidoDevice{}
	}
	return devs
}

func (state *AppState) GetFidoDeviceByCredentialID(credentialID []byte) *FidoDevice {
	if state == nil || state.Inner == nil || len(credentialID) == 0 {
		return nil
	}
	db := state.GetDB()
	if db == nil {
		return nil
	}
	dev, err := db.GetFidoDeviceByCredentialID(credentialID)
	if err != nil {
		return nil
	}
	return dev
}

func (state *AppState) SaveFidoDevice(device *FidoDevice) error {
	if state == nil || state.Inner == nil || device == nil || device.Username == "" {
		return nil
	}
	lowerName := strings.ToLower(device.Username)
	device.Username = lowerName
	if db := state.GetDB(); db != nil {
		return db.SaveFidoDevice(device)
	}
	return ErrDatabaseUnavailable
}

func (state *AppState) DeleteFidoDevice(username, deviceID string) error {
	if state == nil || state.Inner == nil || username == "" || deviceID == "" {
		return nil
	}
	lowerName := strings.ToLower(username)
	db := state.GetDB()
	if db == nil {
		return ErrDatabaseUnavailable
	}
	return db.DeleteFidoDevice(lowerName, deviceID)
}

func (state *AppState) DeleteFidoDevicesByUsername(username string) error {
	if state == nil || state.Inner == nil || username == "" {
		return nil
	}
	lowerName := strings.ToLower(username)
	if db := state.GetDB(); db != nil {
		return db.DeleteFidoDevicesByUsername(lowerName)
	}
	return ErrDatabaseUnavailable
}

func (state *AppState) UpdateFidoSignCount(credentialID []byte, signCount uint32) error {
	if state == nil || state.Inner == nil || len(credentialID) == 0 {
		return nil
	}
	if db := state.GetDB(); db != nil {
		return db.UpdateFidoSignCount(credentialID, signCount)
	}
	return ErrDatabaseUnavailable
}

func (state *AppState) UpdateFidoDeviceState(credentialID []byte, signCount uint32, backupState bool, backupEligible bool) error {
	if state == nil || state.Inner == nil || len(credentialID) == 0 {
		return nil
	}
	if db := state.GetDB(); db != nil {
		return db.UpdateFidoDeviceState(credentialID, signCount, backupState, backupEligible)
	}
	return ErrDatabaseUnavailable
}
