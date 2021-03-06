/*
This chaincode provides a very simplistic shared ledger view of cross border
financial transactions. Its main purpose is to experiment with the hyperledger
fabric blockchain service on IBM Bluemix.
*/
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/iShamSLam/chaincode/model"

	"github.com/hyperledger/fabric/core/chaincode/shim" // v0.6
)

var (
	// passport chaincode application logger
	logger = shim.NewLogger("passport-chaincode")
	// mapping of chaincode handler functions
	handlerMap = NewHandlerMap()
)

func main() {
	initLogging()
	cc := new(Chaincode)
	cc.registerHandlers()
	err := shim.Start(cc)
	if err != nil {
		logger.Errorf("Error starting chaincode: %s", err)
	}
}

// Chaincode Chaincode shim method receiver struct
type Chaincode struct{}

//------------------------
// Chaincode API functions
//------------------------

// Init called to initialize the chaincode
func (cc *Chaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return nil, nil
}

// Invoke chaincode interface implementation
func (cc *Chaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return cc.handleInvocation(stub, function, args)
}

// Query chaincode interface implementation
func (cc *Chaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return cc.handleInvocation(stub, function, args)
}

func (cc *Chaincode) handleInvocation(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	logger.Debugf("Invoking chaincode handler function %s with args %v", function, args)

	res, err := handlerMap.Handle(stub, function, args)
	if err != nil {
		logger.Errorf("Error when calling handler for function %s. Error: %s", function, err)
	}
	return res, err
}

//------------------
// Handler functions
//------------------

// GetAccountList query blockchain accounts by customer ID
func (cc *Chaincode) GetAccountList(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering GetAccountList with args %v", args)

	if len(args) == 0 {
		return nil, errors.New("Missing required customer ID")
	}
	customerID := args[0]
	// Query state using partial keys
	keysIter, err := cc.partialCompositeKeyQuery(stub, model.AccountObjectType, []string{customerID})
	if err != nil {
		logger.Errorf("Failed to get account list. Error: %s", err)
		return nil, err
	}
	accountList := model.AccountList{}
	for keysIter.HasNext() {
		_, accountBytes, _ := keysIter.Next()
		acc := new(model.Account)
		if err := json.Unmarshal(accountBytes, acc); err != nil {
			logger.Errorf("Failed to get account details. Error: %s", err)
			continue
		}
		accountList.Accounts = append(accountList.Accounts, acc)
	}
	jsonList, _ := json.Marshal(accountList)
	logger.Debugf("Returning account list: %s", jsonList)
	return jsonList, nil
}

// GetAccount query blockchain account by account ID
func (cc *Chaincode) GetAccount(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering GetAccount with args %v", args)

	if len(args) != 2 {
		return nil, errors.New("Missing required customer ID and / or account ID")
	}

	customerID := args[0]
	accountID := args[1]

	key, _ := cc.createCompositeKey(model.AccountObjectType, []string{customerID, accountID})
	accountBytes, err := stub.GetState(key)
	if err != nil {
		logger.Errorf("Failed to get account details. Error: %s", err)
		return nil, err
	}
	return accountBytes, nil
}

// OpenAccount opens an account, store into chaincode state as a JSON record
func (cc *Chaincode) OpenAccount(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering OpenAccount with args %v", args)

	if len(args) == 0 {
		return nil, errors.New("Missing required account data JSON")
	}

	account, err := model.CreateAccount([]byte(args[0]))
	if err != nil {
		logger.Errorf("Error when creating new account. Error: %s", err)
		return nil, fmt.Errorf("Error creating new account. Error: %s", err)
	}
	key, _ := cc.createCompositeKey(account.GetObjectType(), []string{account.CustomerID, account.ID})
	accountData, _ := json.Marshal(account)
	stub.PutState(key, accountData)

	return accountData, nil
}

// TopupAccount update account balance
func (cc *Chaincode) TopupAccount(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering TopupAccount with args %v", args)

	if len(args) != 3 {
		return nil, errors.New("Missing required input arguments")
	}

	accountData, err := cc.GetAccount(stub, args[:2])
	if err != nil {
		return nil, err
	}
	if accountData == nil {
		return nil, fmt.Errorf("Account with number %s not found.", args[1])
	}
	account := new(model.Account)
	bytesToStruct([]byte(accountData), account)
	amount, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Error parsing amount value %s", args[2])
	}
	account.Credit(amount)
	key, _ := cc.createCompositeKey(account.GetObjectType(), []string{account.CustomerID, account.ID})
	accountData, _ = json.Marshal(account)
	stub.PutState(key, accountData)

	return accountData, nil
}

// CloseAccount closes the given account
func (cc *Chaincode) CloseAccount(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering CloseAccount with args %v", args)

	if len(args) != 2 {
		return nil, errors.New("Missing required customer ID and / or account ID")
	}

	accountData, err := cc.GetAccount(stub, args[:2])
	if err != nil {
		return nil, err
	}
	if accountData == nil {
		return nil, fmt.Errorf("Account with number %s not found.", args[1])
	}

	account := new(model.Account)
	bytesToStruct(accountData, account)
	account.Closed = true
	key, _ := cc.createCompositeKey(account.GetObjectType(), []string{account.CustomerID, account.ID})
	accountData, _ = json.Marshal(account)
	stub.PutState(key, accountData)

	return accountData, nil
}

// TransferMoney transfer money
func (cc *Chaincode) TransferMoney(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering with args %v", args)

	if len(args) == 0 {
		return nil, errors.New("Missing transfer details JSON")
	}
	transferData := args[0]
	t := new(model.Transfer)
	bytesToStruct([]byte(transferData), t)
	if err := t.Validate(); err != nil {
		return nil, err
	}
	accountData, err := cc.GetAccount(stub, []string{t.FromCustomerID, t.FromAccountID})
	if err != nil {
		return nil, err
	}
	if accountData == nil {
		return nil, fmt.Errorf("Account with number %s not found.", t.FromAccountID)
	}
	fromAccount := new(model.Account)
	bytesToStruct(accountData, fromAccount)
	accountData, err = cc.GetAccount(stub, []string{t.ToCustomerID, t.ToAccountID})
	if err != nil {
		return nil, err
	}
	if accountData == nil {
		return nil, fmt.Errorf("Account with number %s not found.", t.ToAccountID)
	}
	toAccount := new(model.Account)
	bytesToStruct(accountData, toAccount)

	if fromAccount.Closed {
		cc.recordTransaction(stub, fromAccount.CustomerID, fromAccount.ID, t, model.AccountClosed, model.Failed)
		return nil, fmt.Errorf("Cannot transfer money from closed account %s", t.FromAccountID)
	}

	if toAccount.Closed {
		cc.recordTransaction(stub, toAccount.CustomerID, toAccount.ID, t, model.AccountClosed, model.Failed)
		return nil, fmt.Errorf("Cannot transfer money into closed account %s", t.ToAccountID)
	}

	if fromAccount.Balance-t.Amount < 0 {
		cc.recordTransaction(stub, fromAccount.CustomerID, fromAccount.ID, t, model.InsufficientFunds, model.Failed)
		return nil, fmt.Errorf("Insufficient funds available in account %s", t.FromAccountID)
	}

	cc.debitAccount(stub, fromAccount, t.Amount+t.Fee)
	cc.recordTransaction(stub, fromAccount.CustomerID, fromAccount.ID, t, "", model.Debited)
	cc.creditAccount(stub, toAccount, t.Amount)
	cc.recordTransaction(stub, toAccount.CustomerID, toAccount.ID, t, "", model.Credited)

	return nil, nil
}

// GetTransactionList query blockchain accounts by account ID
func (cc *Chaincode) GetTransactionList(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering with args %v", args)

	if len(args) != 2 {
		return nil, errors.New("Missing required customer ID and / or account ID")
	}

	customerID := args[0]
	accountID := args[1]

	// Query state using partial keys
	keysIter, err := cc.partialCompositeKeyQuery(stub, model.TransactionObjectType, []string{customerID, accountID})
	if err != nil {
		logger.Errorf("Failed to get transaction list. Error: %s", err)
		return nil, err
	}
	tranList := model.TransactionList{}
	for keysIter.HasNext() {
		_, txnBytes, _ := keysIter.Next()
		txn := new(model.Transaction)
		if err := json.Unmarshal(txnBytes, txn); err != nil {
			logger.Errorf("Failed to get transaction details. Error: %s", err)
			continue
		}
		tranList.Transactions = append(tranList.Transactions, txn)
	}
	sort.Sort(sort.Reverse(model.ByCreated(tranList.Transactions)))
	jsonList, _ := json.Marshal(tranList)
	logger.Debugf("Returning transaction list: %s", jsonList)
	return jsonList, nil
}

// GetTransaction query blockchain transaction by transaction ID
func (cc *Chaincode) GetTransaction(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering with args %v", args)

	if len(args) != 3 {
		return nil, errors.New("Missing required account ID and / or transaction ID")
	}

	customerID := args[0]
	accountID := args[1]
	tranID := args[2]

	key, _ := cc.createCompositeKey(model.TransactionObjectType, []string{customerID, accountID, tranID})
	txnBytes, err := stub.GetState(key)
	if err != nil {
		logger.Errorf("Failed to get transaction details. Error: %s", err)
		return nil, err
	}
	return txnBytes, nil
}

func (cc *Chaincode) recordTransaction(stub shim.ChaincodeStubInterface, customerID string, accountID string, t *model.Transfer, code model.TxFailureCode, status model.TxStatus) error {
	txn, _ := model.CreateTransaction(customerID, accountID, t, code, status)
	txnData, err := json.Marshal(txn)
	if err != nil {
		return fmt.Errorf("Error marshalling transaction data. Error: %s", err)
	}
	key, _ := cc.createCompositeKey(txn.GetObjectType(), []string{txn.CustomerID, txn.AccountID, txn.ID})
	stub.PutState(key, txnData)
	return nil
}

func (cc *Chaincode) debitAccount(stub shim.ChaincodeStubInterface, a *model.Account, amount int64) error {
	a.Debit(amount)
	accountData, _ := json.Marshal(a)
	key, _ := cc.createCompositeKey(a.GetObjectType(), []string{a.CustomerID, a.ID})
	stub.PutState(key, accountData)
	return nil
}

func (cc *Chaincode) creditAccount(stub shim.ChaincodeStubInterface, a *model.Account, amount int64) error {
	a.Credit(amount)
	accountData, _ := json.Marshal(a)
	key, _ := cc.createCompositeKey(a.GetObjectType(), []string{a.CustomerID, a.ID})
	stub.PutState(key, accountData)
	return nil
}

//-------------------------------------------------
// Helpers
//-------------------------------------------------
func initLogging() {
	logger.SetLevel(shim.LogInfo)
	logLevel, _ := shim.LogLevel(os.Getenv("SHIM_LOGGING_LEVEL"))
	shim.SetLoggingLevel(logLevel)
}

// Registers handler function mappings
func (cc *Chaincode) registerHandlers() {
	handlerMap.Add("OpenAccount", cc.OpenAccount)
	handlerMap.Add("CloseAccount", cc.CloseAccount)
	handlerMap.Add("GetAccount", cc.GetAccount)
	handlerMap.Add("GetAccountList", cc.GetAccountList)
	handlerMap.Add("TransferMoney", cc.TransferMoney)
	handlerMap.Add("TopupAccount", cc.TopupAccount)
	handlerMap.Add("GetTransaction", cc.GetTransaction)
	handlerMap.Add("GetTransactionList", cc.GetTransactionList)
}

// Helper functions

func (cc *Chaincode) createCompositeKey(objectType string, attributes []string) (string, error) {
	const minKeyValue = "0"
	key := objectType + minKeyValue
	for _, att := range attributes {
		key += att + minKeyValue
	}
	logger.Debugf("Created composite key: %s", key)
	return key, nil
}

func (cc *Chaincode) partialCompositeKeyQuery(stub shim.ChaincodeStubInterface, objectType string, keys []string) (shim.StateRangeQueryIteratorInterface, error) {
	partialCompositeKey, _ := cc.createCompositeKey(objectType, keys)
	keysIter, err := stub.RangeQueryState(partialCompositeKey, partialCompositeKey+string(utf8.MaxRune))
	if err != nil {
		return nil, fmt.Errorf("Error fetching rows: %s", err)
	}
	return keysIter, nil
}

// bytesToStruct unmarshals byte slice into given data type
func bytesToStruct(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		logger.Errorf("Error unmarshalling data for type %T", v)
		return err
	}
	return nil
}
