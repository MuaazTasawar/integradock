package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- Domain models ----

type Product struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	SKU      string  `json:"sku"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

type Order struct {
	ID        string    `json:"id"`
	ProductID string    `json:"product_id"`
	Quantity  int       `json:"quantity"`
	Status    string    `json:"status"` // placed | cancelled
	CreatedAt time.Time `json:"created_at"`
}

type CreateOrderRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// ---- In-memory store ----

type store struct {
	mu       sync.Mutex
	products map[string]*Product
	orders   map[string]*Order
	nextOrd  int
}

func newStore() *store {
	s := &store{
		products: map[string]*Product{
			"prod_001": {ID: "prod_001", Name: "Wireless Mouse", SKU: "WM-100", Price: 19.99, Quantity: 150},
			"prod_002": {ID: "prod_002", Name: "Mechanical Keyboard", SKU: "MK-200", Price: 79.99, Quantity: 60},
			"prod_003": {ID: "prod_003", Name: "USB-C Hub", SKU: "UCH-300", Price: 34.50, Quantity: 200},
		},
		orders:  map[string]*Order{},
		nextOrd: 1,
	}
	return s
}

// ---- Handlers ----

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *store) listProducts(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Product, 0, len(s.products))
	for _, p := range s.products {
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *store) getProduct(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.products[id]
	if !ok {
		writeErr(w, http.StatusNotFound, "product not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *store) createOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProductID == "" || req.Quantity <= 0 {
		writeErr(w, http.StatusBadRequest, "product_id and a positive quantity are required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	product, ok := s.products[req.ProductID]
	if !ok {
		writeErr(w, http.StatusNotFound, "product not found")
		return
	}
	if product.Quantity < req.Quantity {
		writeErr(w, http.StatusConflict, "insufficient stock")
		return
	}

	product.Quantity -= req.Quantity

	id := fmt.Sprintf("ord_%03d", s.nextOrd)
	s.nextOrd++
	order := &Order{
		ID:        id,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
		Status:    "placed",
		CreatedAt: time.Now().UTC(),
	}
	s.orders[id] = order

	writeJSON(w, http.StatusCreated, order)
}

func (s *store) listOrders(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Order, 0, len(s.orders))
	for _, o := range s.orders {
		out = append(out, o)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *store) getOrder(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		writeErr(w, http.StatusNotFound, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *store) cancelOrder(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		writeErr(w, http.StatusNotFound, "order not found")
		return
	}
	if o.Status == "cancelled" {
		writeErr(w, http.StatusConflict, "order already cancelled")
		return
	}
	o.Status = "cancelled"
	if product, pok := s.products[o.ProductID]; pok {
		product.Quantity += o.Quantity
	}
	writeJSON(w, http.StatusOK, o)
}

// ---- Routing ----

func main() {
	s := newStore()
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "integradock-mock-api"})
	})

	mux.HandleFunc("/products", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "only GET is supported on /products")
			return
		}
		s.listProducts(w, r)
	})

	mux.HandleFunc("/products/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/products/")
		if id == "" {
			writeErr(w, http.StatusBadRequest, "product id is required")
			return
		}
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "only GET is supported on /products/{id}")
			return
		}
		s.getProduct(w, r, id)
	})

	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.listOrders(w, r)
		case http.MethodPost:
			s.createOrder(w, r)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "only GET and POST are supported on /orders")
		}
	})

	mux.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/orders/")
		if id == "" {
			writeErr(w, http.StatusBadRequest, "order id is required")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.getOrder(w, r, id)
		case http.MethodDelete:
			s.cancelOrder(w, r, id)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "only GET and DELETE are supported on /orders/{id}")
		}
	})

	port := "9090"
	log.Printf("mock-api: listening on :%s", port)
	if err := http.ListenAndServe(":"+port, logRequests(mux)); err != nil {
		log.Fatalf("mock-api: server stopped: %v", err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// unused import guard removed at compile time by Go tooling if strconv unused later phases
var _ = strconv.Itoa
