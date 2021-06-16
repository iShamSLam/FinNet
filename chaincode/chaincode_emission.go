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


// Update emission balance
func (cc *Chaincode) Emission(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering Emission with args %v", args)

	if len(args) != 3 {
		return nil, errors.New("Missing required input arguments")
	}

	privateCollection, err := cc.GetCollection(stub, args[:2])
	if err != nil {
		return nil, err
	}
	if privateCollection == nil {
		return nil, fmt.Errorf("No access to gossip collection", args[2])
	}
	transaction := new(model.Transactions)
	bytesToStruct([]byte(args[1]), transaction)
	amount, err := strconv.ParseInt(args[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Error parsing amount value %s", args[2])
	}
	transaction.Write(amount)
	key, _ := cc.createCompositeKey(transaction.GetObjectType(), []string{transaction.Amount, privateCollection})
	privateCollection, _ = json.Marshal(transaction)
	stub.PutState(key, transaction)

	return transaction, nil
}

// Get last amount
func (cc *Chaincode) GetEmissionAmount(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	logger.Debugf("Entering with args %v", args)

	if len(args) != 2 {
		return nil, errors.New("Missing required arguments")
	}

	privateCollection := args[0]

	// Query state using partial keys
	keysIter, err := cc.partialCompositeKeyQuery(stub, model.TransactionObjectType, []string{ privateCollection })
	if err != nil {
		logger.Errorf("Failed to get collection. Error: %s", err)
		return nil, err
	}
	tranList := model.TransactionList{}
	for keysIter.HasNext() {
		_, txnBytes, _ := keysIter.Next()
		txn := new(model.Transaction)
		if err := json.Unmarshal(txnBytes, txn); err != nil {
			logger.Errorf("Failed to get collection. Error: %s", err)
			continue
		}
		tranList.Transactions = append(tranList.Transactions, txn)
	}
	sort.Sort(sort.Reverse(model.ByCreated(tranList.Transactions)))
	jsonList, _ := json.Marshal(tranList)
	logger.Debugf("Returning transaction list: %s", jsonList)
	return jsonList, nil
}

