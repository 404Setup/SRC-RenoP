/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

// MaxLegalDocumentBytes bounds each administrator-authored Markdown document.
const MaxLegalDocumentBytes = 512 << 10

// LegalConfig owns the instance's legal documents and cookie notice.
type LegalConfig struct {
	PrivacyPolicy  string `json:"privacy_policy" yaml:"privacy_policy"`
	TermsOfService string `json:"terms_of_service" yaml:"terms_of_service"`
	LegalNotice    string `json:"legal_notice" yaml:"legal_notice"`
	CookieBanner   bool   `json:"cookie_banner" yaml:"cookie_banner"`
	revision       string
}

const defaultPrivacyPolicy = `Last updated: January 1, 2026

This Privacy Policy explains how this RenoP repository instance collects, uses, stores, and protects your information when you access or use this service.

## 1. Information We Collect

### Account Information
When you register an account or authenticate with this instance, we may collect:
- Username and display name
- Email address (primary and verified provider emails)
- Hashed and salted authentication credentials (passwords are never stored in plaintext)
- Multi-factor authentication credentials (TOTP secrets, WebAuthn / Passkey public credentials)
- Profile links and optional biography information

### Package and Repository Data
When you publish, manage, or download packages:
- Package metadata, version manifests, and release descriptions
- Package archives, tarballs, container image layers, and manifests
- Cryptographic signatures and public keys (e.g. GPG, PGP)
- Team memberships, repository access permissions, and transfer records

### Technical and Usage Information
To ensure service availability, security, and integrity, we automatically record:
- IP addresses, request timestamps, and HTTP request headers (such as User-Agent)
- Scoped API token identifiers and authorization logs
- Audit logs for security-relevant actions (such as credential updates, role changes, and publication events)
- Browser cookie preferences and legal consent timestamps

## 2. How We Use Information

We process your data strictly for the following operational and security purposes:
- **Service Provision:** Authenticating users, resolving package dependencies, and distributing artifacts for Cargo, npm, Docker, and Maven ecosystems.
- **Security and Integrity:** Protecting against unauthorized access, credential abuse, spam, denial-of-service, and malicious package publications.
- **Auditing and Compliance:** Recording tamper-evident audit logs of administrative and package mutation operations.
- **Notification:** Sending essential transactional emails (e.g., account confirmation, password recovery, ticket notifications).

## 3. Third-Party Services and Cookies

- **Essential Cookies:** We use session cookies and security tokens strictly necessary for authentication and CSRF protection.
- **Security Verification (CAPTCHA):** When enabled, third-party verification services (such as Cloudflare Turnstile, Google reCAPTCHA, hCaptcha, or Friendly Captcha) may be loaded only with your explicit consent or configuration.
- **Third-Party Identity Providers:** If you choose to log in via OAuth (e.g., GitHub, GitLab, Google, Microsoft), we receive your provider identity ID and verified email as permitted by your authorization.

## 4. Data Retention and Account Lifecycle

- Account data is retained for the active lifetime of your account.
- In accordance with repository integrity standards, published public packages and release records may be preserved in an immutable or deprecated state to prevent breaking downstream software builds.
- If you retire or delete your account, personal credentials are permanently erased, and associated email reservations are released after a cooldown period. Audit logs are retained for a limited retention window before automated pruning.

## 5. Your Rights and Contact

Depending on your jurisdiction, you may have rights to access, rectify, export, or request deletion of your personal data. To exercise your rights or submit privacy-related inquiries, please contact the administrator of this RenoP instance.
`

const defaultTermsOfService = `Last updated: January 1, 2026

Welcome to RenoP. By creating an account, publishing packages, or accessing services provided by this repository instance, you agree to comply with and be bound by the following Terms of Service.

## 1. Acceptance of Terms

By accessing or using this instance, you acknowledge that you have read, understood, and agree to be bound by these Terms and the associated Privacy Policy. If you do not agree with these Terms, you must immediately discontinue use of this service.

## 2. Account Security and Responsibilities

- You are responsible for safeguarding your login credentials, multi-factor authentication devices, and scoped API tokens.
- You must immediately notify the instance administrator if you suspect any unauthorized access or security compromise of your account.
- Each user is responsible for all activities and package publications occurring under their account.

## 3. Acceptable Use Policy

You agree not to use this service to:
- Upload, publish, or distribute malware, spyware, ransomware, cryptominers, trojans, or intentionally backdoored software.
- Engage in unauthorized scanning, probing, vulnerability exploitation, or denial-of-service attacks against this instance or downstream users.
- Impersonate any individual, project, organization, or brand, or intentionally squat on popular package names or namespaces to deceive users.
- Violate any applicable local, national, or international laws, export regulations, or third-party intellectual property rights.

## 4. Package Publication and Licensing

- **License and Rights:** By publishing packages or documentation to this instance, you represent and warrant that you hold all necessary rights and licenses to distribute such content.
- **Immutability and Stability:** To maintain dependency stability across the software ecosystem, published package releases and artifact checksums are generally immutable and may not be removed once publicly distributed, except where mandated by law, security advisory, or administrative intervention.
- **Deprecation and Tombstones:** Packages may be marked as deprecated or unlisted in accordance with repository lifecycle rules while preserving historical resolution for existing projects.

## 5. Service Availability and Moderation

- The instance operator reserves the right, at its sole discretion, to lock, suspend, deprecate, or remove packages, repositories, teams, or accounts that violate these Terms or threaten system security.
- The service is provided on an "AS IS" and "AS AVAILABLE" basis without warranties of any kind, either express or implied.
- To the maximum extent permitted by applicable law, the instance operator shall not be liable for any direct, indirect, incidental, or consequential damages arising from the use or inability to use this service.

## 6. Modifications to Terms

The instance operator reserves the right to update these Terms at any time. Material changes will be indicated by an updated revision date, and continued use of the instance following notice of revisions constitutes acceptance of the updated Terms.
`

const defaultLegalNotice = `## Service Operator Information

This service is operated by the instance administrator. Please update this document with your organization or individual legal disclosure.

- **Operator / Entity Name:** [Instance Operator / Organization Name]
- **Contact Email:** [contact@example.com]
- **Mailing Address:** [Operator Physical Address]
- **Responsible Person:** [Administrator / Data Protection Officer]

## Content Disclaimer

This repository hosts software packages, source code, and container images authored and submitted by registered users and third-party upstream mirrors. While reasonable measures are taken to investigate reported violations, the operator assumes no liability for the content, correctness, or licensing of user-submitted artifacts.
`

// DefaultLegalConfig supplies preset legal documents and cookie notice defaults.
func DefaultLegalConfig() LegalConfig {
	value := LegalConfig{
		PrivacyPolicy:  defaultPrivacyPolicy,
		TermsOfService: defaultTermsOfService,
		LegalNotice:    defaultLegalNotice,
		CookieBanner:   true,
	}
	value.updateRevision()
	return value
}

func (value *LegalConfig) updateRevision() {
	digest := sha256.Sum256([]byte(value.PrivacyPolicy + "\x00" + value.TermsOfService))
	value.revision = hex.EncodeToString(digest[:])
}

// Normalize validates content and restores placeholder documents when a field is empty.
func (value *LegalConfig) Normalize() error {
	defaults := DefaultLegalConfig()
	for _, entry := range []struct {
		content  *string
		fallback string
	}{
		{&value.PrivacyPolicy, defaults.PrivacyPolicy},
		{&value.TermsOfService, defaults.TermsOfService},
		{&value.LegalNotice, defaults.LegalNotice},
	} {
		if strings.TrimSpace(*entry.content) == "" {
			*entry.content = entry.fallback
		}
		if len(*entry.content) > MaxLegalDocumentBytes || !utf8.ValidString(*entry.content) || strings.ContainsRune(*entry.content, 0) {
			return errors.New("legal document must contain UTF-8 text within 512 KiB")
		}
	}
	value.updateRevision()
	return nil
}

// Revision identifies the privacy policy and terms accepted by a browser.
func (value LegalConfig) Revision() string {
	if value.revision == "" {
		value.updateRevision()
	}
	return value.revision
}

// DeepCopy returns an independently owned legal configuration snapshot.
func (value LegalConfig) DeepCopy() LegalConfig {
	value.PrivacyPolicy = strings.Clone(value.PrivacyPolicy)
	value.TermsOfService = strings.Clone(value.TermsOfService)
	value.LegalNotice = strings.Clone(value.LegalNotice)
	return value
}

func (value *LegalConfig) UnmarshalJSON(data []byte) error {
	*value = DefaultLegalConfig()
	type alias LegalConfig
	if err := json.Unmarshal(data, (*alias)(value)); err != nil {
		return err
	}
	return value.Normalize()
}

func (value *LegalConfig) UnmarshalYAML(node *yaml.Node) error {
	*value = DefaultLegalConfig()
	type alias LegalConfig
	if err := node.Decode((*alias)(value)); err != nil {
		return err
	}
	return value.Normalize()
}
