/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import "testing"

type contentTypeTestCase struct {
	ext  string
	want string
}

func TestContentTypeByExt(t *testing.T) {
	cases := []contentTypeTestCase{
		{ext: ".css", want: "text/css; charset=utf-8"},
		{ext: "CSS", want: "text/css; charset=utf-8"},
		{ext: ".js", want: "text/javascript; charset=utf-8"},
		{ext: ".png", want: "image/png"},
		{ext: ".PNG", want: "image/png"},
		{ext: ".pom", want: "text/xml; charset=utf-8"},
		{ext: ".json", want: "application/json; charset=utf-8"},
		{ext: ".jar", want: "application/java-archive"},
		{ext: "", want: "application/octet-stream"},
		{ext: ".unknownxyz", want: "application/octet-stream"},
	}
	for _, tc := range cases {
		if got := ContentTypeByExt(tc.ext); got != tc.want {
			t.Fatalf("ContentTypeByExt(%q)=%q want %q", tc.ext, got, tc.want)
		}
	}
}
