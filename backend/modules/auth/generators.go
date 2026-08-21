package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

// generateOTPCode: 4 digit angka ("0000"-"9999", padded) -- disepakati 4 digit + expired 5
// menit buat sekarang (belum ada provider WA/SMS, OTP dicek langsung dari database pas dev).
func generateOTPCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d", n.Int64()), nil
}

// generateSessionToken: 32 byte random, di-hex-in -- token session, disimpen mobile_member_session.token.
func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateMemberCode: prefix "MOB" + sequence 4 digit (MOB0001, MOB0002, ...) -- KHUSUS buat
// member yang daftar sendiri lewat mobile app (beda dari generateMemberCode() di sudocore2
// yang prefix-nya dari nama member_type / "GEN" -- di sini gak collect member_type pas daftar,
// dan sengaja dikasih prefix beda biar ketauan dari code-nya asal member ini dari mobile).
func generateMemberCode(ctx context.Context, db *bun.DB) (string, error) {
	var existingCodes []string
	if err := db.NewRaw(`SELECT code FROM master_member WHERE code LIKE 'MOB%'`).Scan(ctx, &existingCodes); err != nil {
		return "", err
	}

	maxSeq := 0
	for _, code := range existingCodes {
		suffix := strings.TrimPrefix(code, "MOB")
		if len(suffix) != 4 {
			continue
		}
		seq, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}

	return fmt.Sprintf("MOB%04d", maxSeq+1), nil
}

var validPin = regexp.MustCompile(`^[0-9]{6}$`)

// hashPin: bcrypt, cost default -- PIN gak pernah disimpen plaintext. Divalidasi dulu 6 digit
// angka SEBELUM di-hash (disepakati 6 digit, sama pola app finansial kayak OVO/GoPay).
func hashPin(pin string) (string, error) {
	if !validPin.MatchString(pin) {
		return "", fmt.Errorf("pin harus 6 digit angka")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// comparePin: cocokin PIN plaintext ke hash yang kesimpen -- balikin false kalau gak cocok,
// TANPA ngebedain error teknis vs salah PIN (bcrypt.CompareHashAndPassword balikin error buat
// keduanya, disamain jadi bool doang di sini biar pemanggil gak perlu bedain).
func comparePin(pin, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pin)) == nil
}
