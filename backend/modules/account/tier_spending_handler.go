package account

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
)

type tierSettingRow struct {
	Type         string  `bun:"type"`
	TypeWeekDay  *string `bun:"type_week_day"`
	TypeMonthDay *int    `bun:"type_month_day"`
}

type currentTierRow struct {
	Level         int     `bun:"tier_level"`
	Name          *string `bun:"tier_name"`
	StyleTemplate *string `bun:"tier_style_template"`
}

type tierSpendingResponse struct {
	TypeEvaluation string   `json:"type_evaluation"`
	NextEvaluation string   `json:"next_evaluation"`
	PeriodStart    string   `json:"period_start"`
	PeriodEnd      string   `json:"period_end"`
	SpendingTotal  string   `json:"spending_total"`
	Tier           tierInfo `json:"tier"`
}

type tierInfo struct {
	Level         int     `json:"level"`
	Name          *string `json:"name"`
	StyleTemplate *string `json:"style_template"`
}

// TierSpending: posisi tier member yang lagi login + progress spending periode berjalan +
// jadwal evaluasi berikutnya -- DIGABUNG jadi 1 endpoint (2026-08-21, sebelumnya sempet
// direncanain kepisah dari /me) karena tier/spending/next_evaluation itu satu concern yang
// sama, basisnya sama-sama dari master_member_tier_setting -- masuk akal ditampilin bareng di
// 1 layar "progress tier", beda dari /me yang murni profil statis.
//
// `period_start`/`period_end`/`spending_total` -- lihat computePeriodStart(), window PERIODE BERJALAN
// (anchor ke type_week_day/type_month_day sampai HARI INI), BEDA dari window "mundur N hari/
// bulan" yang dipakai background job membertierevaluation (itu buat EVALUASI, ini buat DISPLAY).
//
// `next_evaluation` -- tanggal evaluasi BERIKUTNYA (bisa hari ini sendiri kalau hari ini
// kebetulan cocok config & belum lewat jam 01:00), lihat computeNextEvaluation().
func (h *handler) TierSpending(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var setting tierSettingRow
	err := h.db.NewRaw(`
		SELECT type, type_week_day, type_month_day FROM master_member_tier_setting WHERE id = 1
	`).Scan(c.Context(), &setting)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("belum ada setting tier"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil setting tier"))
	}

	now := time.Now()
	periodStart, err := computePeriodStart(setting, now)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage(err.Error()))
	}
	nextEvaluation, err := computeNextEvaluation(setting, now)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage(err.Error()))
	}
	periodEnd := now.Format("2006-01-02")

	var spending string
	err = h.db.NewRaw(`
		SELECT COALESCE(SUM(total_billing), 0) FROM pos_order
		WHERE status = 'paid' AND member_id = ?
		  AND order_out >= ?::date AND order_out < (?::date + interval '1 day')
	`, memberID, periodStart, periodEnd).Scan(c.Context(), &spending)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data spending"))
	}

	var currentTier currentTierRow
	err = h.db.NewRaw(`
		SELECT mm.tier_level, mmtsd.name AS tier_name, mmtsd.style_template AS tier_style_template
		FROM master_member mm
		LEFT JOIN master_member_tier_setting_detail mmtsd ON mmtsd.level = mm.tier_level
		WHERE mm.id = ?
	`, memberID).Scan(c.Context(), &currentTier)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data tier"))
	}

	return c.JSON(res.Success().SetData(tierSpendingResponse{
		TypeEvaluation: setting.Type,
		NextEvaluation: nextEvaluation,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		SpendingTotal:  spending,
		Tier: tierInfo{
			Level:         currentTier.Level,
			Name:          currentTier.Name,
			StyleTemplate: currentTier.StyleTemplate,
		},
	}))
}

// computePeriodStart: type=week -> tanggal terdekat ke belakang (termasuk hari ini kalau
// cocok) yang nama harinya = type_week_day. type=month -> type_month_day BULAN INI kalau
// tanggal hari ini udah nyampe/lewat, kalau belum -> type_month_day BULAN KEMARIN.
func computePeriodStart(s tierSettingRow, now time.Time) (string, error) {
	switch s.Type {
	case "week":
		if s.TypeWeekDay == nil {
			return "", errors.New("setting tier belum lengkap (type_week_day kosong)")
		}
		target := strings.ToLower(*s.TypeWeekDay)
		for i := 0; i < 7; i++ {
			d := now.AddDate(0, 0, -i)
			if strings.ToLower(d.Weekday().String()) == target {
				return d.Format("2006-01-02"), nil
			}
		}
		return "", errors.New("type_week_day tidak valid")
	case "month":
		if s.TypeMonthDay == nil {
			return "", errors.New("setting tier belum lengkap (type_month_day kosong)")
		}
		day := *s.TypeMonthDay
		if now.Day() >= day {
			return time.Date(now.Year(), now.Month(), day, 0, 0, 0, 0, now.Location()).Format("2006-01-02"), nil
		}
		prevMonth := now.AddDate(0, -1, 0)
		return time.Date(prevMonth.Year(), prevMonth.Month(), day, 0, 0, 0, 0, now.Location()).Format("2006-01-02"), nil
	default:
		return "", errors.New("setting tier tidak valid")
	}
}

// computeNextEvaluation: kebalikan computePeriodStart, nyari MAJU. type=week -> tanggal
// terdekat ke depan (termasuk hari ini kalau cocok) yang nama harinya = type_week_day.
// type=month -> type_month_day bulan ini kalau belum kelewat, kalau udah kelewat -> bulan depan.
func computeNextEvaluation(s tierSettingRow, now time.Time) (string, error) {
	switch s.Type {
	case "week":
		if s.TypeWeekDay == nil {
			return "", errors.New("setting tier belum lengkap (type_week_day kosong)")
		}
		target := strings.ToLower(*s.TypeWeekDay)
		for i := 0; i < 7; i++ {
			d := now.AddDate(0, 0, i)
			if strings.ToLower(d.Weekday().String()) == target {
				return d.Format("2006-01-02"), nil
			}
		}
		return "", errors.New("type_week_day tidak valid")
	case "month":
		if s.TypeMonthDay == nil {
			return "", errors.New("setting tier belum lengkap (type_month_day kosong)")
		}
		day := *s.TypeMonthDay
		if now.Day() <= day {
			return time.Date(now.Year(), now.Month(), day, 0, 0, 0, 0, now.Location()).Format("2006-01-02"), nil
		}
		nextMonth := now.AddDate(0, 1, 0)
		return time.Date(nextMonth.Year(), nextMonth.Month(), day, 0, 0, 0, 0, now.Location()).Format("2006-01-02"), nil
	default:
		return "", errors.New("setting tier tidak valid")
	}
}
