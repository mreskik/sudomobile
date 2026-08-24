package config

import "os"

// StoragePath: root direktori file upload (gambar dll). Default "./storage" (folder sendiri,
// standalone) -- tapi kalau sudomobile di-deploy sepanggung sama sudocore2 (server/disk yang
// sama), di-set lewat env STORAGE_PATH ke folder storage sudocore2 LANGSUNG (mount, bukan
// copy/download) -- sama pola kayak APIANDORDER (SUDOCORE_STORAGE_PATH di
// APIANDORDER/backend/router.go). Efeknya SATU storage dipakai bareng: gambar admin-managed
// (master_image_mb_cust dkk) otomatis kebaca sudomobile tanpa proxy/download HTTP, dan foto
// profil member yang di-upload lewat sudomobile ikut numpang di folder yang sama -- struktur
// path-nya emang udah dibikin identik dari awal (storage/uploads/images/<uuid>.<ext>, lihat
// modules/account/photo_handler.go), jadi gak perlu migrasi/rename apa-apa buat gabung.
var StoragePath = "./storage"

func InitStoragePath() {
	if p := os.Getenv("STORAGE_PATH"); p != "" {
		StoragePath = p
	}
}
