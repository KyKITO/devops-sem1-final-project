package main

import (
	"archive/zip"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

const (
	dbUser     = "validator"
	dbPassword = "val1dat0r"
	dbName     = "project-sem-1"
	dbHost     = "localhost"
	dbPort     = "5432"
)

type postResponse struct {
	TotalItems      int     `json:"total_items"`
	TotalCategories int     `json:"total_categories"`
	TotalPrice      float64 `json:"total_price"`
}

var db *sql.DB

func main() {
	var err error
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Fail to connect to DB", err)
	}
	defer db.Close()

	http.HandleFunc("/api/v0/prices", handleReq)
	log.Println("Server has started on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleReq(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		handlePost(w, r)
	case "GET":
		handleGet(w, r)
	default:
		http.Error(w, "Unsupported method", http.StatusMethodNotAllowed)
	}
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	log.Println("Got POST-request.")

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("Fail to load the file: %v\n", err)
		http.Error(w, "Fail to load the file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	log.Printf("File %s uploaded sucessfully.\n", header.Filename)

	tempDir := "./temp"
	os.MkdirAll(tempDir, os.ModePerm)

	zipPath := filepath.Join(tempDir, header.Filename)
	outFile, err := os.Create(zipPath)
	if err != nil {
		log.Printf("Fail to create outFile %s: %v\n", zipPath, err)
		http.Error(w, "Fail to create outFile", http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, file)
	if err != nil {
		log.Printf("Fail to save file: %v\n", err)
		http.Error(w, "Fail to save file", http.StatusInternalServerError)
		return
	}
	log.Printf("File saved in temp dir: %s\n", zipPath)

	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		log.Printf("Fail to read the archive: %v\n", err)
		http.Error(w, "Fail to read the archive", http.StatusInternalServerError)
		return
	}
	defer zipReader.Close()

	var totalItems int
	var totalPrice float64
	var totalCategories int
	categories := make(map[string]bool)

	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".csv") {
			log.Printf("CSV detected: %s\n", f.Name)
			processCSV(f, &totalItems, &totalPrice, &totalCategories)
		}
	}

	response := postResponse{
		TotalItems:      totalItems,
		TotalCategories: len(categories),
		TotalPrice:      totalPrice,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("JSON error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
	}

}

type dataRow struct {
	ProductID int
	CreatedAt string
	Name      string
	Category  string
	Price     float64
}

func processCSV(f *zip.File, totalItems *int, totalPrice *float64, totalCategories *int) {
	log.Printf("Starting CSV: %s\n", f.Name)

	rc, err := f.Open()
	if err != nil {
		log.Printf("Fail to open CSV %s: %v\n", f.Name, err)
		return
	}
	defer rc.Close()

	reader := csv.NewReader(rc)

	header, err := reader.Read()
	if err != nil {
		log.Printf("Fail to read CSV header: %v\n", err)
		return
	}
	log.Printf("CSV Header: %v\n", header)

	var rows []dataRow

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Fail to read row in CSV: %v\n", err)
			return
		}

		productID, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err != nil {
			log.Printf("Invalid product_id '%s': %v\n", record[0], err)
			return
		}

		createdAt := strings.TrimSpace(record[4])
		name := strings.TrimSpace(record[1])
		category := strings.TrimSpace(record[2])

		price, err := strconv.ParseFloat(strings.TrimSpace(record[3]), 64)
		if err != nil {
			log.Printf("Invalid price '%s': %v\n", record[3], err)
			return
		}

		rows = append(rows, dataRow{
			ProductID: productID,
			CreatedAt: createdAt,
			Name:      name,
			Category:  category,
			Price:     price,
		})

	}

	if len(rows) == 0 {
		log.Println("No valid rows found, skipping database insertion.")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Fail to begin transaction: %v\n", err)
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	stmt, err := tx.Prepare("INSERT INTO prices (created_at, name, category, price) VALUES ($1, $2, $3, $4, $5)")
	if err != nil {
		log.Printf("Fail to prepare statement: %v\n", err)
		return
	}
	defer stmt.Close()

	for _, row := range rows {
		_, err = stmt.Exec(row.ProductID, row.CreatedAt, row.Name, row.Category, row.Price)
		if err != nil {
			log.Printf("Error inserting into DB ID %d: %v\n", row.ProductID, err)
			return
		}
	}

	*totalItems, *totalCategories, *totalPrice, err = calculateStatistics(tx, *totalItems)
	if err != nil {
		log.Printf("Fail to calculate statistics: %v\n", err)
	}
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query("SELECT id, created_at, name, category, price FROM prices")
	defer rows.Close()

	tempDir := "./temp"
	os.MkdirAll(tempDir, os.ModePerm)
	csvFilePath := filepath.Join(tempDir, "data.csv")
	csvFile, _ := os.Create(csvFilePath)
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	writer.Write([]string{"id", "created_at", "name", "category", "price"})

	for rows.Next() {
		var id int
		var createdAt, name, category string
		var price int
		if err := rows.Scan(&id, &createdAt, &name, &category, &price); err != nil {
			log.Printf("Failed to scan row: %v\n", err)
			http.Error(w, "Error scanning data", http.StatusInternalServerError)
			return
		}
		writer.Write([]string{strconv.Itoa(id), createdAt, name, category, strconv.Itoa(price)})
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating over rows: %v\n", err)
		http.Error(w, "Error retrieving data", http.StatusInternalServerError)
		return
	}

	writer.Flush()

	zipFilePath := filepath.Join(tempDir, "data.zip")
	zipFile, _ := os.Create(zipFilePath)
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	fileInZip, _ := zipWriter.Create("data.csv")
	csvBytes, _ := os.ReadFile(csvFilePath)
	fileInZip.Write(csvBytes)
	zipWriter.Close()

	w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, zipFilePath)
}

func calculateStatistics(tx *sql.Tx, totalItems int) (int, int, float64, error) {
	var totalCategories int
	var totalPrice float64

	query := `
	  SELECT COUNT(DISTINCT category), COALESCE(SUM(price), 0)
	  FROM prices
	`
	err := tx.QueryRow(query).Scan(&totalCategories, &totalPrice)
	if err != nil {
		return 0, 0, 0, err
	}

	return totalItems, totalCategories, totalPrice, nil
}
