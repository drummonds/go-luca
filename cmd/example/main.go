package main

import (
	"fmt"
	"log"
	"time"

	"github.com/drummonds/go-luca"
	_ "github.com/drummonds/go-postgres"
)

func main() {
	ledger, err := luca.NewLedger(":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer ledger.Close()

	// Create accounts (amounts in pence with exponent -2)
	cash, _ := ledger.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings1, _ := ledger.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.0365) // 3.65% = 10p/day on £1000
	savings2, _ := ledger.CreateAccount("Liability:Savings:0002", "GBP", -2, 0.05)   // 5%

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Initial capital injection: Equity → Cash (100000.00 = 10000000 pence)
	ledger.RecordMovement(equity.ID, cash.ID, 10000000, jan1, "Initial capital")

	// Customer deposits
	ledger.RecordMovement(cash.ID, savings1.ID, 1000000, jan1, "Customer 1 deposit") // 10000.00
	ledger.RecordMovement(cash.ID, savings2.ID, 500000, jan1, "Customer 2 deposit")  // 5000.00

	// Linked movements: purchase with VAT
	purchases, _ := ledger.CreateAccount("Expense:Purchases", "GBP", -2, 0)
	vatInput, _ := ledger.CreateAccount("Asset:VATInput", "GBP", -2, 0)
	ledger.RecordLinkedMovements([]luca.MovementInput{
		{FromAccountID: cash.ID, ToAccountID: purchases.ID, Amount: 50000, Description: "Office supplies"},       // 500.00
		{FromAccountID: cash.ID, ToAccountID: vatInput.ID, Amount: 10000, Description: "VAT on office supplies"}, // 100.00
	}, time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC))

	// Run daily interest for January
	ledger.EnsureInterestAccounts()
	fmt.Println("=== Daily Interest ===")
	for day := 1; day <= 31; day++ {
		date := time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
		results, err := ledger.RunDailyInterest(date)
		if err != nil {
			log.Fatalf("interest day %d: %v", day, err)
		}
		if day == 1 || day == 15 || day == 31 {
			for _, r := range results {
				acct, _ := ledger.GetAccountByID(r.AccountID)
				balDec := luca.IntToDecimal(r.OpeningBalance, acct.Exponent)
				intDec := luca.IntToDecimal(r.InterestAmount, acct.Exponent)
				fmt.Printf("  Day %2d | %-30s | balance=%s | interest=%s\n",
					day, acct.FullPath, balDec.StringFixed(2), intDec.StringFixed(2))
			}
		}
	}

	// Final balances
	fmt.Println("\n=== Final Balances ===")
	accounts := []*luca.Account{cash, equity, savings1, savings2}
	for _, acct := range accounts {
		bal, _ := ledger.Balance(acct.ID)
		balDec := luca.IntToDecimal(bal, acct.Exponent)
		fmt.Printf("  %-35s  %10s GBP\n", acct.FullPath, balDec.StringFixed(2))
	}

	// Hierarchical reporting
	endOfJan := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	liabBal, liabExp, _ := ledger.BalanceByPath("Liability", endOfJan)
	assetBal, assetExp, _ := ledger.BalanceByPath("Asset", endOfJan)
	liabDec := luca.IntToDecimal(liabBal, liabExp)
	assetDec := luca.IntToDecimal(assetBal, assetExp)
	fmt.Printf("\n  Total Liabilities:  %10s GBP\n", liabDec.StringFixed(2))
	fmt.Printf("  Total Assets:       %10s GBP\n", assetDec.StringFixed(2))
}
