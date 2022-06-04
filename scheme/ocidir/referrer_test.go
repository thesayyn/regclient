package ocidir

import (
	"context"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
	"github.com/sirupsen/logrus"
)

func TestReferrer(t *testing.T) {
	// setup http server with and without API support
	ctx := context.Background()
	fsOS := rwfs.OSNew("")
	fsMem := rwfs.MemNew()
	err := rwfs.CopyRecursive(fsOS, "../../testdata", fsMem, ".")
	if err != nil {
		t.Errorf("failed to setup memfs copy: %v", err)
		return
	}
	log := &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}
	o := New(
		WithFS(fsMem),
		WithLog(log),
	)
	repo := "ocidir://testrepo"
	tagName := "v3"
	aType := "sbom"
	extraAnnot := "org.opencontainers.artifact.sbom.format"
	extraValue := "SPDX json"
	digest1 := digest.FromString("example1")
	digest2 := digest.FromString("example2")
	mRef, err := ref.New(repo + ":" + tagName)
	if err != nil {
		t.Errorf("failed to parse ref %s: %v", repo+":"+tagName, err)
		return
	}
	m, err := o.ManifestGet(ctx, mRef)
	if err != nil {
		t.Errorf("failed to get manifest: %v", err)
	}
	// artifact being attached
	artifactAnnot := map[string]string{
		annotType:  aType,
		extraAnnot: extraValue,
	}
	mDesc := m.GetDescriptor()
	artifact := v1.Manifest{
		Versioned: v1.ManifestSchemaVersion,
		MediaType: types.MediaTypeOCI1Manifest,
		Config: types.Descriptor{
			MediaType: types.MediaTypeOCI1ImageConfig,
			Size:      8,
			Digest:    digest1,
		},
		Layers: []types.Descriptor{
			{
				MediaType: types.MediaTypeOCI1LayerGzip,
				Size:      8,
				Digest:    digest2,
			},
		},
		Annotations: artifactAnnot,
		Refers:      &mDesc,
	}
	artifactM, err := manifest.New(manifest.WithOrig(artifact))
	if err != nil {
		t.Errorf("failed creating artifact manifest: %v", err)
	}
	artifactBody, err := artifactM.RawBody()
	if err != nil {
		t.Errorf("failed extracting raw body from artifact: %v", err)
	}

	// attach to v1 image
	t.Run("Put", func(t *testing.T) {
		r, err := ref.New(repo + ":" + tagName)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
		}
		err = o.ReferrerPut(ctx, r, artifactM)
		if err != nil {
			t.Errorf("Failed running ReferrerPut: %v", err)
			return
		}
	})

	// list referrers to v1
	t.Run("List", func(t *testing.T) {
		r, err := ref.New(repo + ":" + tagName)
		if err != nil {
			t.Errorf("Failed creating getRef: %v", err)
			return
		}
		rl, err := o.ReferrerList(ctx, r)
		if err != nil {
			t.Errorf("Failed running ReferrerList: %v", err)
			return
		}
		if len(rl.Descriptors) <= 0 {
			t.Errorf("descriptor list missing")
			return
		}
		if rl.Descriptors[0].MediaType != types.MediaTypeOCI1Manifest ||
			rl.Descriptors[0].Size != int64(len(artifactBody)) ||
			rl.Descriptors[0].Digest != artifactM.GetDescriptor().Digest ||
			!mapStringStringEq(rl.Descriptors[0].Annotations, artifactAnnot) {
			t.Errorf("returned descriptor mismatch: %v", rl.Descriptors[0])
		}
	})
}

func mapStringStringEq(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}