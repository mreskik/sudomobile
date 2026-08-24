package account

import (
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"sudomobile/backend/config"
	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	maxPhotoSize   = 2 * 1024 * 1024 // 2MB -- sama persis konvensi maxImageSize di sudocore2 (backend/modules/upload/upload_service.go)
	maxPhotoPerDay = 3
)

// photoStorageRoot: subfolder tempat foto profil disimpen, RELATIF ke config.StoragePath --
// "uploads/images" biar strukturnya identik sama upload_service.go sudocore2
// (storage/uploads/images/<uuid>.<ext>), disengaja biar bisa numpang di storage yang sama
// (lihat backend/config/storage.go) tanpa perlu subfolder terpisah/rename apa-apa.
func photoStorageRoot() string {
	return filepath.Join(config.StoragePath, "uploads", "images")
}

var allowedPhotoExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

type updatePhotoResponse struct {
	ProfilePhotoSrc string `json:"profile_photo_src"`
}

// UpdatePhoto: ganti foto profil member yang lagi login -- PROTECTED, member_id dari session
// token. Storage SENDIRI di sudomobile (bukan numpang ke sudocore2/upload) -- self-contained,
// gak nambah dependency HTTP antar-service baru. Konvensi limit/ekstensi file SENGAJA disamain
// persis modul upload sudocore2 (2MB, jpg/jpeg/png/webp/gif) walau kode-nya duplikat (beda
// module Go, gak bisa saling import package internal).
//
// Barrier max 3x ganti PER HARI (bukan per prosesnya doang) -- dicek dari
// mobile_member_photo_change_log, COUNT baris created_at >= hari ini. Sengaja dihitung dari
// HARI KALENDER (bukan rolling 24 jam), lebih predictable buat user ("besok reset").
func (h *handler) UpdatePhoto(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var todayCount int
	err := h.db.NewRaw(`
		SELECT COUNT(*) FROM mobile_member_photo_change_log
		WHERE member_id = ? AND created_at >= CURRENT_DATE
	`, memberID).Scan(c.Context(), &todayCount)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cek batas ganti foto"))
	}
	if todayCount >= maxPhotoPerDay {
		return c.JSON(res.SetCode(100).SetMessage(fmt.Sprintf("batas ganti foto profil hari ini udah abis (maks %dx), coba lagi besok", maxPhotoPerDay)))
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("file wajib diisi"))
	}

	path, err := savePhoto(c, fh)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage(err.Error()))
	}

	tx, err := h.db.BeginTx(c.Context(), nil)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal simpan foto"))
	}
	gagal := true
	defer func() {
		if gagal {
			tx.Rollback()
		}
	}()

	if _, err := tx.NewRaw(
		`UPDATE master_member SET profile_photo_src = ? WHERE id = ?`, path, memberID,
	).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal simpan foto"))
	}
	if _, err := tx.NewRaw(
		`INSERT INTO mobile_member_photo_change_log (member_id) VALUES (?)`, memberID,
	).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal simpan foto"))
	}

	gagal = false
	if err := tx.Commit(); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal simpan foto"))
	}

	return c.JSON(res.Success().SetData(updatePhotoResponse{ProfilePhotoSrc: path}))
}

func savePhoto(c fiber.Ctx, fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxPhotoSize {
		return "", fmt.Errorf("ukuran file maksimal %d MB", maxPhotoSize/1024/1024)
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !allowedPhotoExt[ext] {
		return "", errors.New("tipe file tidak diizinkan")
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	root := photoStorageRoot()
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}

	filename := id.String() + ext
	dest := filepath.Join(root, filename)
	if err := c.SaveFile(fh, dest); err != nil {
		return "", err
	}

	// URL yang dibalikin (disimpen ke DB) SENGAJA dibangun terpisah dari dest (path fisik di
	// disk) -- dest bisa aja nunjuk keluar folder ini (config.StoragePath = "../sudocore2/storage"
	// pas storage digabung), tapi klien selalu akses lewat route "/storage/*" yang sama gak
	// peduli StoragePath fisiknya di mana. Kalau ikutan filepath.ToSlash(dest), path
	// "../sudocore2/storage/..." bakal bocor jadi URL yang salah.
	return "/storage/uploads/images/" + filename, nil
}
