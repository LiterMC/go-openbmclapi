/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
 * All rights reserved
 *
 *  This program is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU Affero General Public License as published
 *  by the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU Affero General Public License for more details.
 *
 *  You should have received a copy of the GNU Affero General Public License
 *  along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

const repoName = "LiterMC/go-openbmclapi"
const lastetReleaseEndPoint = "https://api.github.com/repos/" + repoName + "/releases/latest"
const cdnURL = "https://cdn.crashmc.com/"

type GithubRelease struct {
	Tag     releaseVersion `json:"tag_name"`
	HtmlURL string         `json:"html_url"`
	Body    string         `json:"body"`
}

func (cr *Cluster) checkUpdate() (err error) {
	if currentBuildTag == nil {
		return
	}

	log.Info("Checking for Go-OpenBmclAPI latest release")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	req, err := http.NewRequest(http.MethodGet, lastetReleaseEndPoint, nil)
	if err != nil {
		return
	}
	if config.GithubAPI.Authorization != "" {
		req.Header.Set("Authorization", config.GithubAPI.Authorization)
	}
	var resp *http.Response
	{
		tctx, cancel := context.WithTimeout(ctx, time.Second*10)
		resp, err = cr.cachedCli.Do(req.WithContext(tctx))
		cancel()
	}
	if err != nil {
		if req, err = http.NewRequest(http.MethodGet, cdnURL+lastetReleaseEndPoint, nil); err != nil {
			return
		}
		tctx, cancel := context.WithTimeout(ctx, time.Second*10)
		resp, err = cr.cachedCli.Do(req.WithContext(tctx))
		cancel()
		if err != nil {
			return
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return utils.NewHTTPStatusErrorFromResponse(resp)
	}
	release := new(GithubRelease)
	if err = json.NewDecoder(resp.Body).Decode(release); err != nil {
		return
	}
	if !currentBuildTag.Less(&release.Tag) {
		return
	}
	// TODO: print all middle change logs
	log.Infof(Tr("info.update.detected"), release.Tag, currentBuildTag)
	log.Infof(Tr("info.update.changelog"), currentBuildTag, release.Tag, release.Body)
	cr.notifyManager.OnUpdateAvaliable(release)
	return
}

type releaseVersion struct {
	Major, Minor, Patch int
	Build               int
}

var currentBuildTag = func() (v *releaseVersion) {
	v = new(releaseVersion)
	if v.UnmarshalText(([]byte)(build.BuildVersion)) != nil {
		return nil
	}
	return
}()

func (v *releaseVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d-%d", v.Major, v.Minor, v.Patch, v.Build)
}

func (v *releaseVersion) UnmarshalJSON(data []byte) (err error) {
	var s string
	if err = json.Unmarshal(data, &s); err != nil {
		return
	}
	return v.UnmarshalText(([]byte)(s))
}

func (v *releaseVersion) UnmarshalText(data []byte) (err error) {
	data, _ = bytes.CutPrefix(data, ([]byte)("v"))
	data, build, _ := bytes.Cut(data, ([]byte)("-"))
	if v.Build, err = strconv.Atoi((string)(build)); err != nil {
		return
	}
	vers := bytes.Split(data, ([]byte)("."))
	if len(vers) != 3 {
		return fmt.Errorf("Unexpected release tag format %q", vers)
	}
	if v.Major, err = strconv.Atoi((string)(vers[0])); err != nil {
		return
	}
	if v.Minor, err = strconv.Atoi((string)(vers[1])); err != nil {
		return
	}
	if v.Patch, err = strconv.Atoi((string)(vers[2])); err != nil {
		return
	}
	return
}

func (v *releaseVersion) Less(w *releaseVersion) bool {
	return v.Major < w.Major || v.Major == w.Major && (v.Minor < w.Minor || v.Minor == w.Minor && (v.Patch < w.Patch || v.Patch == w.Patch && (v.Build < w.Build)))
}
