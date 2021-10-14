package schema

import (
	"database/sql"

	"github.com/GuiaBolso/darwin"
)

var migrations = []darwin.Migration{
	{
		Version:     1,
		Description: "Add Suppliers",
		Script: `
		CREATE TABLE suppliers (
			id char(36) NOT NULL PRIMARY KEY,
			company_id char(36) NOT NULL,
			code CHAR(10) NOT NULL,
			name VARCHAR(45) NOT NULL UNIQUE,
			address VARCHAR(255) NOT NULL,
			phone VARCHAR(20) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			created_by char(36) NOT NULL,
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_by char(36) NOT NULL,
			UNIQUE(company_id, code)
		);`,
	},
	{
		Version:     2,
		Description: "Add Purchases",
		Script: `
		CREATE TABLE purchases (
			id char(36) NOT NULL PRIMARY KEY,
			company_id	char(36) NOT NULL,
			branch_id char(36) NOT NULL,
			branch_name varchar(100) NOT NULL,
			supplier_id char(36) NOT NULL,
			code	CHAR(13) NOT NULL,
			purchase_date	DATE NOT NULL,
			remark VARCHAR(255) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			created_by char(36) NOT NULL,
			updated_by char(36) NOT NULL,
			UNIQUE(company_id, code),
			CONSTRAINT fk_purchases_to_suppliers FOREIGN KEY (supplier_id) REFERENCES suppliers(id)
		);`,
	},
	{
		Version:     3,
		Description: "Add Purchase Details",
		Script: `
		CREATE TABLE purchase_details (
			id char(36) NOT NULL PRIMARY KEY,
			purchase_id	char(36) NOT NULL,
			product_id char(36) NOT NULL,
			quantity INT NOT NULL CHECK (quantity > 0),
			CONSTRAINT fk_purchase_details_to_purchases FOREIGN KEY (purchase_id) REFERENCES purchases(id) ON DELETE CASCADE ON UPDATE CASCADE
		);`,
	},
	{
		Version:     4,
		Description: "Add Purchase Return",
		Script: `
		CREATE TABLE purchase_returns (
			id char(36) NOT NULL PRIMARY KEY,
			company_id	char(36) NOT NULL,
			branch_id char(36) NOT NULL,
			branch_name varchar(100) NOT NULL,
			purchase_id char(36) NOT NULL,
			code	CHAR(13) NOT NULL,
			return_date	DATE NOT NULL,
			remark VARCHAR(255) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			created_by char(36) NOT NULL,
			updated_by char(36) NOT NULL,
			UNIQUE(company_id, code),
			CONSTRAINT fk_purchase_returns_to_purchases FOREIGN KEY (purchase_id) REFERENCES purchases(id)
		);`,
	},
	{
		Version:     5,
		Description: "Add Purchase Return Details",
		Script: `
		CREATE TABLE purchase_return_details (
			id char(36) NOT NULL PRIMARY KEY,
			purchase_return_id	char(36) NOT NULL,
			product_id char(36) NOT NULL,
			quantity INT NOT NULL CHECK (quantity > 0),
			CONSTRAINT fk_purchase_return_details_to_purchase_returns FOREIGN KEY (purchase_return_id) REFERENCES purchase_returns(id) ON DELETE CASCADE ON UPDATE CASCADE
		);`,
	},
}

func Migrate(db *sql.DB) error {
	driver := darwin.NewGenericDriver(db, darwin.PostgresDialect{})

	d := darwin.New(driver, migrations, nil)

	return d.Migrate()
}
