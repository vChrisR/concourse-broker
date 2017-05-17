// This file was generated by counterfeiter
package resourceserverfakes

import (
	"sync"

	"github.com/concourse/atc/api/resourceserver"
	"github.com/concourse/atc/radar"
)

type FakeScannerFactory struct {
	NewResourceScannerStub        func(db radar.RadarDB) radar.Scanner
	newResourceScannerMutex       sync.RWMutex
	newResourceScannerArgsForCall []struct {
		db radar.RadarDB
	}
	newResourceScannerReturns struct {
		result1 radar.Scanner
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeScannerFactory) NewResourceScanner(db radar.RadarDB) radar.Scanner {
	fake.newResourceScannerMutex.Lock()
	fake.newResourceScannerArgsForCall = append(fake.newResourceScannerArgsForCall, struct {
		db radar.RadarDB
	}{db})
	fake.recordInvocation("NewResourceScanner", []interface{}{db})
	fake.newResourceScannerMutex.Unlock()
	if fake.NewResourceScannerStub != nil {
		return fake.NewResourceScannerStub(db)
	} else {
		return fake.newResourceScannerReturns.result1
	}
}

func (fake *FakeScannerFactory) NewResourceScannerCallCount() int {
	fake.newResourceScannerMutex.RLock()
	defer fake.newResourceScannerMutex.RUnlock()
	return len(fake.newResourceScannerArgsForCall)
}

func (fake *FakeScannerFactory) NewResourceScannerArgsForCall(i int) radar.RadarDB {
	fake.newResourceScannerMutex.RLock()
	defer fake.newResourceScannerMutex.RUnlock()
	return fake.newResourceScannerArgsForCall[i].db
}

func (fake *FakeScannerFactory) NewResourceScannerReturns(result1 radar.Scanner) {
	fake.NewResourceScannerStub = nil
	fake.newResourceScannerReturns = struct {
		result1 radar.Scanner
	}{result1}
}

func (fake *FakeScannerFactory) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.newResourceScannerMutex.RLock()
	defer fake.newResourceScannerMutex.RUnlock()
	return fake.invocations
}

func (fake *FakeScannerFactory) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ resourceserver.ScannerFactory = new(FakeScannerFactory)
