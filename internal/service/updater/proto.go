/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import "renop/pkg/pb"

// ToPbUpdateState converts domain UpdateState to the wire protobuf message.
func ToPbUpdateState(s *UpdateState) *pb.UpdateState {
	if s == nil {
		return nil
	}
	return &pb.UpdateState{
		Status:             s.Status,
		LatestVersion:      s.LatestVersion,
		DownloadUrl:        s.DownloadUrl,
		Progress:           int32(s.Progress),
		ErrorMessage:       s.ErrorMessage,
		Size:               s.Size,
		EstimatedDiskSpace: s.EstimatedDiskSpace,
		ReleaseDate:        s.ReleaseDate,
		ReleaseNotes:       s.ReleaseNotes,
		CommitSha:          s.CommitSha,
		IsRelease:          s.IsRelease,
	}
}
