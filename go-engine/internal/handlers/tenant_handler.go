package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MuaazTasawar/integradock/go-engine/internal/models"
)

// TenantHandler owns all tenant-related HTTP endpoints.
type TenantHandler struct {
	DB *pgxpool.Pool
}

func NewTenantHandler(db *pgxpool.Pool) *TenantHandler {
	return &TenantHandler{DB: db}
}

// Create inserts a new tenant. POST /api/tenants
func (h *TenantHandler) Create(c *fiber.Ctx) error {
	var req models.CreateTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Name == "" || req.Slug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name and slug are required"})
	}

	ctx := context.Background()
	var tenant models.Tenant
	row := h.DB.QueryRow(ctx, `
		INSERT INTO tenants (name, slug)
		VALUES ($1, $2)
		RETURNING id, name, slug, created_at, updated_at
	`, req.Name, req.Slug)

	if err := row.Scan(&tenant.ID, &tenant.Name, &tenant.Slug, &tenant.CreatedAt, &tenant.UpdatedAt); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create tenant: " + err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(tenant)
}

// List returns all tenants. GET /api/tenants
func (h *TenantHandler) List(c *fiber.Ctx) error {
	ctx := context.Background()
	rows, err := h.DB.Query(ctx, `
		SELECT id, name, slug, created_at, updated_at FROM tenants ORDER BY created_at DESC
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list tenants: " + err.Error()})
	}
	defer rows.Close()

	tenants := make([]models.Tenant, 0)
	for rows.Next() {
		var t models.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to scan tenant: " + err.Error()})
		}
		tenants = append(tenants, t)
	}

	return c.JSON(tenants)
}

// GetBySlug returns a single tenant by slug. GET /api/tenants/:slug
func (h *TenantHandler) GetBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")
	ctx := context.Background()

	var t models.Tenant
	row := h.DB.QueryRow(ctx, `
		SELECT id, name, slug, created_at, updated_at FROM tenants WHERE slug = $1
	`, slug)

	if err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant not found"})
	}

	return c.JSON(t)
}
