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

// MemberID: helper baca member_id yang udah divalidasi & disimpen Auth ke locals.
func MemberID(c fiber.Ctx) int64 {
	id, _ := c.Locals(memberIDLocalsKey).(int64)
	return id
}
