package middleware

import (
	"strings"

	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

const memberIDLocalsKey = "member_id"

// Auth: validasi session token (hasil Register/LoginOTP) dari header `Authorization: Bearer
// <token>` -- sama pola BranchTokenAuth di APIANDORDER, cuma di sini tokennya scoped ke
// customer (mobile_member_session), bukan ke branch. Token gak ada/invalid/expired -> ditolak,
// member_id valid -> disimpen ke locals, diambil lewat middleware.MemberID(c).
func Auth(db *bun.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		res := helpers.NewResponse()

		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return c.JSON(res.SetCode(100).SetMessage("token tidak ditemukan"))
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			return c.JSON(res.SetCode(100).SetMessage("token tidak ditemukan"))
		}

		var memberID int64
		err := db.NewRaw(
			`SELECT member_id FROM mobile_member_session WHERE token = ? AND expires_at > now()`,
			token,
		).Scan(c.Context(), &memberID)
		if err != nil {
			return c.JSON(res.SetCode(100).SetMessage("token tidak valid"))
		}

		c.Locals(memberIDLocalsKey, memberID)
		return c.Next()
	}
}

// MemberID: helper baca member_id yang udah divalidasi & disimpen Auth/OptionalAuth ke locals.
func MemberID(c fiber.Ctx) int64 {
	id, _ := c.Locals(memberIDLocalsKey).(int64)
	return id
}

// OptionalAuth: BEDA dari Auth -- token BOLEH gak dikirim/invalid/expired, request TETAP
// lanjut (c.Next()) sebagai guest (MemberID(c) balik 0), gak pernah nolak request di titik ini.
// Kalau token ADA dan valid, member_id-nya di-resolve & disimpen ke locals SAMA PERSIS Auth,
// jadi MemberID(c) di handler tetap keisi normal buat request yang login.
//
// Dibikin 2026-09-23 -- gap dari perubahan 2026-09-22 yang geser endpoint /order jadi publik
// (Authorization opsional) TAPI middleware Auth-nya kadung dicopot total dari grup route itu
// (Auth sifatnya hard-reject kalau token kosong/invalid, gak cocok buat publik) tanpa ada
// pengganti yang tetep nyoba resolve token kalau ada. Akibatnya member_id SELALU 0 walau
// client kirim token valid -- mb_order.member_id gak pernah kesimpen, DAN guard promo ("promo
// tidak bisa dipakai tanpa login") SELALU nolak walau member beneran login. Dipasang gantiin
// "tanpa middleware sama sekali" di orderPublicRouter (router.go).
func OptionalAuth(db *bun.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Next()
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			return c.Next()
		}

		var memberID int64
		err := db.NewRaw(
			`SELECT member_id FROM mobile_member_session WHERE token = ? AND expires_at > now()`,
			token,
		).Scan(c.Context(), &memberID)
		if err != nil {
			// token invalid/expired -- BUKAN error, lanjut sebagai guest (beda dari Auth yang nolak).
			return c.Next()
		}

		c.Locals(memberIDLocalsKey, memberID)
		return c.Next()
	}
}
