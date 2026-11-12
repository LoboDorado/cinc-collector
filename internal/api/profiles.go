package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cinc-project/cinc-collector/internal/auth"
	"github.com/cinc-project/cinc-collector/internal/storage"
	"github.com/cinc-project/cinc-collector/internal/store"
)

// registerProfileRoutes wires the InSpec profile store: where profiles are
// published and where chef-client/InSpec pull them from for compliance
// runs. Metadata lives in SereneDB; tarball bytes live in blob storage
// (local filesystem in dev, S3/MinIO in production) -- see docs/specs for
// why the tarball itself isn't stored in SereneDB.
func (a *API) registerProfileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v0/profiles", a.requireAuth(a.handleUploadProfile))
	mux.HandleFunc("GET /api/v0/profiles", a.requireAuth(a.handleListProfiles))
	mux.HandleFunc("GET /api/v0/profiles/{name}/{version}/tar", a.requireAuth(a.handleDownloadProfile))
	mux.HandleFunc("DELETE /api/v0/profiles/{name}/{version}", a.requireAuth(a.handleDeleteProfile))
}

// handleUploadProfile accepts a profile tarball as the raw request body.
// Required query params: name, version.
func (a *API) handleUploadProfile(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	name := r.URL.Query().Get("name")
	version := r.URL.Query().Get("version")
	if name == "" || version == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameters: name, version")
		return
	}

	tmp, err := os.CreateTemp("", "cinc-collector-profile-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to buffer upload")
		return
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	defer func() { _ = tmp.Close() }()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read upload body")
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rewind upload buffer")
		return
	}

	key := fmt.Sprintf("%s/%s/%s.tar.gz", org, name, version)
	if err := a.opts.Blobs.Put(r.Context(), key, tmp, size); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store profile tarball")
		return
	}

	err = a.opts.Profiles.Create(r.Context(), store.ProfileMetadata{
		Org:        org,
		Name:       name,
		Version:    version,
		SHA256:     hex.EncodeToString(hasher.Sum(nil)),
		StorageKey: key,
		SizeBytes:  size,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store profile metadata")
		return
	}

	writeJSON(w, http.StatusCreated, struct{}{})
}

func (a *API) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	profiles, err := a.opts.Profiles.List(r.Context(), org)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list profiles")
		return
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (a *API) handleDownloadProfile(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	name, version := r.PathValue("name"), r.PathValue("version")

	meta, err := a.opts.Profiles.Get(r.Context(), org, name, version)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up profile")
		return
	}

	blob, err := a.opts.Blobs.Get(r.Context(), meta.StorageKey)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "profile tarball missing from blob storage")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read profile tarball")
		return
	}
	defer func() { _ = blob.Close() }()

	w.Header().Set("Content-Type", "application/x-gzip")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, blob)
}

func (a *API) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	name, version := r.PathValue("name"), r.PathValue("version")

	meta, err := a.opts.Profiles.Get(r.Context(), org, name, version)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up profile")
		return
	}

	if err := a.opts.Profiles.Delete(r.Context(), org, name, version); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete profile metadata")
		return
	}
	if err := a.opts.Blobs.Delete(r.Context(), meta.StorageKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete profile tarball")
		return
	}

	writeJSON(w, http.StatusOK, struct{}{})
}
