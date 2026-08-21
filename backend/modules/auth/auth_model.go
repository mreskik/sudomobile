package auth

import (
	"time"

	"github.com/uptrace/bun"
)

// MobileMemberOTP: kode OTP register/login, phone-based -- lihat migration 100 di sudocore2.
// Tabel prefix "mobile_" (bukan "master_") -- ini bukan master data ERP, tabel pendukung
// khusus modul mobile app. CreatedAt SENGAJA gak dipetain (biarin DB default now() yang isi)
// -- kalau dipetain sebagai time.Time kosong, insert bakal ngirim "0001-01-01" literal
// ngalahin default DB.
type MobileMemberOTP struct {
	bun.BaseModel `bun:"table:mobile_member_otp,alias:mmo"`

	ID          int64      `bun:"id,pk,autoincrement"`
	PhoneNumber string     `bun:"phone_number,notnull"`
	OTPCode     string     `bun:"otp_code,notnull"`
	Type        string     `bun:"type,notnull"` // "register" atau "login"
	ExpiresAt   time.Time  `bun:"expires_at,notnull"`
	VerifiedAt  *time.Time `bun:"verified_at"`
}

// MobileMemberSession: session token hasil Register/LoginOTP/LoginPin, divalidasi
// middleware.Auth -- lihat migration 100 di sudocore2.
type MobileMemberSession struct {
	bun.BaseModel `bun:"table:mobile_member_session,alias:mms"`

	ID        int64     `bun:"id,pk,autoincrement"`
	MemberID  int64     `bun:"member_id,notnull"`
	Token     string    `bun:"token,notnull"`
	ExpiresAt time.Time `bun:"expires_at,notnull"`
}

// MobileMemberPin: PIN login (6 digit, di-hash bcrypt) -- alternatif OTP, 1 member 1 PIN
// (member_id unique). Lihat migration 102 di sudocore2. UpdatedAt SENGAJA gak dipetain di sini
// (di-set manual pas update, sama alasan CreatedAt gak dipetain di model lain).
type MobileMemberPin struct {
	bun.BaseModel `bun:"table:mobile_member_pin,alias:mmp"`

	ID        int64      `bun:"id,pk,autoincrement"`
	MemberID  int64      `bun:"member_id,notnull"`
	PinHash   string     `bun:"pin_hash,notnull"`
	UpdatedAt *time.Time `bun:"updated_at"`
}

// MasterMember: subset kolom master_member (tabel inti ERP, BUKAN mobile-only, makanya tetep
// prefix "master_") yang dibutuhin buat insert dari register -- bukan model lengkap
// (member_type_id/contact_name/email/dst sengaja gak dipetain, kosong/null buat member hasil
// daftar sendiri lewat mobile app).
type MasterMember struct {
	bun.BaseModel `bun:"table:master_member,alias:mm"`

	ID          int64  `bun:"id,pk,autoincrement"`
	Code        string `bun:"code,notnull"`
	Name        string `bun:"name,notnull"`
	PhoneNumber string `bun:"phone_number"`
	IsActive    bool   `bun:"is_active"`
}
