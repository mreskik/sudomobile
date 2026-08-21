package middleware

import (
	"net/url"
	"strconv"

	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

const appSettingHeader = "X-App-Setting"

const (
	dbCodeLocalsKey    = "db_code"
	companyIDLocalsKey = "company_id"
)

// AppSetting: wajib di SEMUA route (termasuk /api/auth/*, bukan cuma yang udah login) --
// header X-App-Setting isinya ciphertext AES-256-GCM (lihat helpers.DecryptAppSetting), yang
// setelah didekripsi bentuknya query-string-style ("db_code=tesmisal&company_id=15"). Sementara
// cuma company_id yang dipakai jadi key (wajib angka & divalidasi eksistensinya ke
// master_company) -- db_code SEMENTARA diabaikan (ditampung ke locals kalau ada, tapi gak
// wajib & gak divalidasi), nunggu jelas db_code itu buat apa.
//
// Response selalu HTTP 200 -- sukses/gagal ditentuin dari `code` di body (0 = sukses, 100 =
// error), sama convention yang dipakai sudocore2/APIANDORDER, bukan HTTP status code.
func AppSetting(db *bun.DB, key []byte) fiber.Handler {
	return func(c fiber.Ctx) error {
		res := helpers.NewResponse()

		raw := c.Get(appSettingHeader)
		if raw == "" {
			return c.JSON(res.SetCode(100).SetMessage(appSettingHeader + " wajib diisi"))
		}

		decrypted, err := helpers.DecryptAppSetting(key, raw)
		if err != nil {
			// pesan error sengaja generic -- gak dibedain "base64 salah" vs "gagal decrypt" vs
			// "auth tag gak cocok", biar gak ngasih informasi ke pihak yang nyoba nebak-nebak
			// format/key-nya.
			return c.JSON(res.SetCode(100).SetMessage(appSettingHeader + " tidak valid"))
		}

		values, err := url.ParseQuery(decrypted)
		if err != nil {
			return c.JSON(res.SetCode(100).SetMessage(appSettingHeader + " format tidak valid"))
		}

		dbCode := values.Get("db_code")

		companyIDRaw := values.Get("company_id")
		if companyIDRaw == "" {
			return c.JSON(res.SetCode(100).SetMessage("company_id wajib diisi di " + appSettingHeader))
		}
		companyID, err := strconv.Atoi(companyIDRaw)
		if err != nil {
			return c.JSON(res.SetCode(100).SetMessage("company_id harus berupa angka"))
		}

		var count int
		if err := db.NewRaw(`SELECT COUNT(*) FROM master_company WHERE id = ?`, companyID).Scan(c.Context(), &count); err != nil {
			return c.JSON(res.SetCode(100).SetMessage("gagal validasi company_id"))
		}
		if count == 0 {
			return c.JSON(res.SetCode(100).SetMessage("company_id tidak ditemukan"))
		}

		c.Locals(dbCodeLocalsKey, dbCode)
		c.Locals(companyIDLocalsKey, companyID)
		return c.Next()
	}
}

// DBCode/CompanyID: helper baca nilai yang udah divalidasi & disimpen AppSetting ke locals --
// dipakai handler lain biar gak nulis type-assertion c.Locals(...) berulang-ulang.
func DBCode(c fiber.Ctx) string {
	code, _ := c.Locals(dbCodeLocalsKey).(string)
	return code
}

func CompanyID(c fiber.Ctx) int {
	id, _ := c.Locals(companyIDLocalsKey).(int)
	return id
}
