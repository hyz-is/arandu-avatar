package avatar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strconv"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-avatar/internal/imaging"
)

// slot is one of the two places a photo is kept for an owner.
type slot string

const (
	slotUpload   slot = "upload"
	slotGravatar slot = "gravatar"
)

// stored is a photo that is on the Disk and whose row is not written yet.
type stored struct {
	slot        slot
	version     string
	contentType string
	sizes       []int
	keys        []string
}

// extensionOf is the extension a content type is kept with.
func extensionOf(contentType string) string {
	if contentType == "image/png" {
		return ".png"
	}
	return ".jpg"
}

// fileKey is where a variant is kept: the row's own identifier, the slot, the
// version and the size. The server writes every part of it; nothing the sender
// typed is in it.
func fileKey(rowID string, s slot, version string, size int, contentType string) string {
	return path.Join("avatar", rowID, string(s)+"-"+version, strconv.Itoa(size)+extensionOf(contentType))
}

// keep puts the variants of a processed photo on the Disk, under the version
// their largest variant makes. It stores nothing when any of them fails.
func (c *core) keep(ctx context.Context, g security.Grant, rowID string, s slot, res imaging.Result) (stored, error) {
	largest := res.Variants[len(res.Variants)-1]
	sum := sha256.Sum256(largest.Body)
	version := hex.EncodeToString(sum[:])[:16]
	out := stored{slot: s, version: version, contentType: res.Type}
	for _, v := range res.Variants {
		key := fileKey(rowID, s, version, v.Size, res.Type)
		if err := c.cfg.Files.Put(ctx, g, key, bytes.NewReader(v.Body), res.Type); err != nil {
			c.drop(ctx, g, out.keys)
			return stored{}, err
		}
		out.keys = append(out.keys, key)
		out.sizes = append(out.sizes, v.Size)
	}
	return out, nil
}

// drop removes files. A failure to remove one is logged and nothing else: the
// file is an orphan nobody can reach, and what asked for the removal has
// already succeeded or failed for a better reason.
func (c *core) drop(ctx context.Context, g security.Grant, keys []string) {
	for _, key := range keys {
		if err := c.cfg.Files.Delete(ctx, g, key); err != nil {
			c.cfg.Logger.Warn("avatar: removing a file that is no longer used", "key", key, "error", err)
		}
	}
}

// keysOf are the files of a slot as the row describes them.
func keysOf(row *Avatar, s slot) []string {
	version, contentType, sizes := row.UploadVersion, row.UploadType, row.UploadSizes
	if s == slotGravatar {
		version, contentType, sizes = row.GravatarVersion, row.GravatarType, row.GravatarSizes
	}
	if version == "" {
		return nil
	}
	var keys []string
	for _, size := range parseSizes(sizes) {
		keys = append(keys, fileKey(row.ID, s, version, size, contentType))
	}
	return keys
}
