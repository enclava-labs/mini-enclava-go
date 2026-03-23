package extract

import "enclava-go/internal/store/sqlite"

func DefaultTemplates() []sqlite.ExtractTemplate {
	return []sqlite.ExtractTemplate{
		{
			ID:           "detailed_invoice",
			Name:         "Detailed Invoice",
			Description:  "Invoice extraction with line items, taxes, and party data",
			SystemPrompt: "You extract structured invoice data. Return only valid JSON.",
			UserPrompt: `Extract invoice data from this document.
Use these optional hints when available:
- company_name: {company_name}
- expected_currency: {currency}

Return JSON with fields:
invoice_number, invoice_date, due_date, service_provider, buyer, subtotal, tax_amount, total_amount, currency, line_items.`,
			ContextSchema: map[string]any{
				"company_name": map[string]any{"type": "string", "required": false},
				"currency":     map[string]any{"type": "string", "required": false},
			},
			IsDefault: true,
			IsActive:  true,
		},
		{
			ID:           "simple_receipt",
			Name:         "Simple Receipt",
			Description:  "Receipt extraction for merchant, date, items, totals",
			SystemPrompt: "You extract structured receipt data. Return only valid JSON.",
			UserPrompt: `Extract receipt data from this document.
Return JSON with fields:
merchant_name, merchant_address, date, items, subtotal, tax, total, currency, payment_method.`,
			ContextSchema: map[string]any{},
			IsDefault:     true,
			IsActive:      true,
		},
		{
			ID:           "expense_report",
			Name:         "Expense Report",
			Description:  "Expense-focused extraction with normalization fields",
			SystemPrompt: "You extract expense data from invoices/receipts. Return only valid JSON.",
			UserPrompt: `Extract expense data from this document.
Optional hints:
- employee_name: {employee_name}
- department: {department}

Return JSON with fields:
vendor, date, amount, currency, category, description, payment_method, tax_amount, is_reimbursable.`,
			ContextSchema: map[string]any{
				"employee_name": map[string]any{"type": "string", "required": false},
				"department":    map[string]any{"type": "string", "required": false},
			},
			IsDefault: true,
			IsActive:  true,
		},
	}
}
