package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"server/db"
	"server/models"
)

func AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	topServices, pieChartData, err := fetchServiceRevenue()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load service revenue analytics"})
		return
	}

	monthlyList, err := fetchMonthlyRevenue()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load monthly revenue analytics"})
		return
	}

	for i := range monthlyList {
		if i > 0 && monthlyList[i-1].Revenue > 0 {
			prev := monthlyList[i-1].Revenue
			curr := monthlyList[i].Revenue
			growth := ((curr - prev) / prev) * 100
			monthlyList[i].Growth = math.Round(growth*100) / 100
		} else {
			monthlyList[i].Growth = 0
		}
	}

	resp := map[string]any{
		"topServices":    topServices,
		"pieChartData":   pieChartData,
		"monthlyRevenue": monthlyList,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func fetchServiceRevenue() ([]models.TopService, []models.ServiceRevenue, error) {
	rows, err := db.DB.Query(`
		SELECT service, SUM(cost)::float8 AS revenue
		FROM queries
		WHERE cost > 0 AND deleted_at IS NULL
		GROUP BY service
		ORDER BY revenue DESC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	topServices := make([]models.TopService, 0)
	pieChartData := make([]models.ServiceRevenue, 0)

	for rows.Next() {
		var service string
		var revenue float64
		if err := rows.Scan(&service, &revenue); err != nil {
			return nil, nil, err
		}

		pieChartData = append(pieChartData, models.ServiceRevenue{Name: service, Value: revenue})
		topServices = append(topServices, models.TopService{Service: service, Revenue: revenue})
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if len(topServices) > 3 {
		topServices = topServices[:3]
	}

	return topServices, pieChartData, nil
}

func fetchMonthlyRevenue() ([]models.MonthlyRevenue, error) {
	rows, err := db.DB.Query(`
		SELECT DATE_TRUNC('month', submitted_at) AS month_start, SUM(cost)::float8 AS revenue
		FROM queries
		WHERE cost > 0 AND deleted_at IS NULL
		GROUP BY 1
		ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	monthlyRevenue := make([]models.MonthlyRevenue, 0)
	for rows.Next() {
		var monthStart time.Time
		var revenue float64
		if err := rows.Scan(&monthStart, &revenue); err != nil {
			return nil, err
		}

		monthlyRevenue = append(monthlyRevenue, models.MonthlyRevenue{
			Month:   monthStart.Format("Jan 2006"),
			Revenue: revenue,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return monthlyRevenue, nil
}
