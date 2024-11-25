/*
 * Cadence - The resource-oriented smart contract programming language
 *
 * Copyright Flow Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package runtime

import (
	"fmt"
	"runtime"
	"sort"

	"github.com/fxamacker/cbor/v2"
	"github.com/onflow/atree"

	"github.com/onflow/cadence/common"
	"github.com/onflow/cadence/common/orderedmap"
	"github.com/onflow/cadence/errors"
	"github.com/onflow/cadence/interpreter"
)

const (
	AccountStorageKey = "stored"
)

type StorageConfig struct {
	StorageFormatV2Enabled bool
}

type storageFormat uint8

const (
	storageFormatUnknown storageFormat = iota
	storageFormatNew
	storageFormatV1
	StorageFormatV2
)

type Storage struct {
	*atree.PersistentSlabStorage

	// cachedDomainStorageMaps is a cache of domain storage maps.
	// Key is StorageKey{address, domain} and value is domain storage map.
	cachedDomainStorageMaps map[interpreter.StorageDomainKey]*interpreter.DomainStorageMap

	// cachedV1Accounts contains the cached result of determining
	// if the account is in storage format v1 or not.
	cachedV1Accounts map[common.Address]bool

	// contractUpdates is a cache of contract updates.
	// Key is StorageKey{contract_address, contract_name} and value is contract composite value.
	contractUpdates *orderedmap.OrderedMap[interpreter.StorageKey, *interpreter.CompositeValue]

	Ledger atree.Ledger

	memoryGauge common.MemoryGauge

	Config StorageConfig

	AccountStorageV1      *AccountStorageV1
	AccountStorageV2      *AccountStorageV2
	scheduledV2Migrations []common.Address
}

var _ atree.SlabStorage = &Storage{}
var _ interpreter.Storage = &Storage{}

func NewStorage(
	ledger atree.Ledger,
	memoryGauge common.MemoryGauge,
	config StorageConfig,
) *Storage {
	decodeStorable := func(
		decoder *cbor.StreamDecoder,
		slabID atree.SlabID,
		inlinedExtraData []atree.ExtraData,
	) (
		atree.Storable,
		error,
	) {
		return interpreter.DecodeStorable(
			decoder,
			slabID,
			inlinedExtraData,
			memoryGauge,
		)
	}

	decodeTypeInfo := func(decoder *cbor.StreamDecoder) (atree.TypeInfo, error) {
		return interpreter.DecodeTypeInfo(decoder, memoryGauge)
	}

	ledgerStorage := atree.NewLedgerBaseStorage(ledger)
	persistentSlabStorage := atree.NewPersistentSlabStorage(
		ledgerStorage,
		interpreter.CBOREncMode,
		interpreter.CBORDecMode,
		decodeStorable,
		decodeTypeInfo,
	)

	accountStorageV1 := NewAccountStorageV1(
		ledger,
		persistentSlabStorage,
		memoryGauge,
	)

	var accountStorageV2 *AccountStorageV2
	if config.StorageFormatV2Enabled {
		accountStorageV2 = NewAccountStorageV2(
			ledger,
			persistentSlabStorage,
			memoryGauge,
		)
	}

	return &Storage{
		Ledger:                ledger,
		PersistentSlabStorage: persistentSlabStorage,
		memoryGauge:           memoryGauge,
		Config:                config,
		AccountStorageV1:      accountStorageV1,
		AccountStorageV2:      accountStorageV2,
	}
}

const storageIndexLength = 8

// GetDomainStorageMap returns existing or new domain storage map for the given account and domain.
func (s *Storage) GetDomainStorageMap(
	inter *interpreter.Interpreter,
	address common.Address,
	domain common.StorageDomain,
	createIfNotExists bool,
) (
	domainStorageMap *interpreter.DomainStorageMap,
) {
	// Get cached domain storage map if it exists.

	domainStorageKey := interpreter.NewStorageDomainKey(s.memoryGauge, address, domain)

	if s.cachedDomainStorageMaps != nil {
		domainStorageMap = s.cachedDomainStorageMaps[domainStorageKey]
		if domainStorageMap != nil {
			return domainStorageMap
		}
	}

	defer func() {
		// Cache domain storage map
		if domainStorageMap != nil {
			s.cacheDomainStorageMap(
				domainStorageKey,
				domainStorageMap,
			)
		}
	}()

	if !s.Config.StorageFormatV2Enabled {

		// When StorageFormatV2 is disabled, handle all accounts as v1 accounts.

		domainStorageMap = s.AccountStorageV1.GetDomainStorageMap(
			address,
			domain,
			createIfNotExists,
		)

		if domainStorageMap != nil {
			s.cacheIsV1Account(address, true)
		}

	} else {

		// StorageFormatV2 is enabled.

		onlyCheckSpecifiedDomainForV1 := !createIfNotExists
		format := s.getAccountStorageFormat(address, domain, onlyCheckSpecifiedDomainForV1)

		switch format {

		case storageFormatUnknown:
			// storageFormatUnknown is returned when !createIfNotExists
			// and domain register doesn't exist.

			if createIfNotExists {
				panic(errors.NewUnreachableError())
			}

			domainStorageMap = nil

		case storageFormatV1:
			domainStorageMap = s.AccountStorageV1.GetDomainStorageMap(
				address,
				domain,
				createIfNotExists,
			)

			s.cacheIsV1Account(address, true)

		case StorageFormatV2, storageFormatNew:
			domainStorageMap = s.AccountStorageV2.GetDomainStorageMap(
				inter,
				address,
				domain,
				createIfNotExists,
			)

			s.cacheIsV1Account(address, false)

		default:
			panic(errors.NewUnreachableError())
		}
	}

	return domainStorageMap
}

// getAccountStorageFormat returns storageFormat for the given account.
// This function determines account format by reading registers.
// If onlyCheckSpecifiedDomainForV1 is true, only the given domain
// register is checked to determine if account format is v1.  If
// domain register doesn't exist, StorageFormatUnknown is returned.
//
// When StorageFormatV2 is disabled:
// - No register reading (accounts are assumed to be in v1 format).
//
// When StorageFormatV2 is enabled:
// - For v2 accounts, "stored" register is read.
//
// - For v1 accounts,
//   - If onlyCheckSpecifiedDomainForV1 is true,
//     "stored" register and given domain register are read.
//   - If onlyCheckSpecifiedDomainForV1 is false and given domain exists,
//     "stored" register and given domain register are read.
//   - If onlyCheckSpecifiedDomainForV1 is false and given domain doesn't exist,
//     "stored" register, given domain register, and all domain registers are read.
//
// - For new accounts, "stored" register, given domain register, and all domain registers are read.
func (s *Storage) getAccountStorageFormat(
	address common.Address,
	domain common.StorageDomain,
	onlyCheckSpecifiedDomainForV1 bool,
) (format storageFormat) {

	// All accounts are assumed to be in v1 format when StorageFormatV2 is disabled.

	if !s.Config.StorageFormatV2Enabled {
		return storageFormatV1
	}

	// Return cached account format (no register reading).

	isCachedV1, isCachedV2 := s.getCachedAccountFormat(address)

	if isCachedV1 {
		return storageFormatV1
	}

	if isCachedV2 {
		return StorageFormatV2
	}

	// Check if account is v2 (by reading "stored" register).

	if s.isV2Account(address) {
		return StorageFormatV2
	}

	// Check if account is v1 (by reading given domain register).

	if s.hasDomainRegister(address, domain) {
		return storageFormatV1
	}

	// Return early if onlyCheckSpecifiedDomainForV1 to prevent more register reading.

	if onlyCheckSpecifiedDomainForV1 {
		return storageFormatUnknown
	}

	// At this point, account is either new account or v1 account without given domain register.

	if s.isV1Account(address) {
		return storageFormatV1
	}

	return storageFormatNew
}

func (s *Storage) getCachedAccountFormat(address common.Address) (format storageFormat, known bool) {
	isV1, cached := s.cachedV1Accounts[address]
	if !cached {
		return storageFormatUnknown, false
	}
	if isV1 {
		return storageFormatV1, true
	} else {
		return StorageFormatV2, true
	}
}

// isV2Account returns true if given account is in account storage format v2.
func (s *Storage) isV2Account(address common.Address) bool {
	accountStorageMapExists, err := hasAccountStorageMap(s.Ledger, address)
	if err != nil {
		panic(err)
	}

	return accountStorageMapExists
}

// hasDomainRegister returns true if given account has given domain register.
// NOTE: account storage format v1 has domain registers.
func (s *Storage) hasDomainRegister(address common.Address, domain common.StorageDomain) (domainExists bool) {
	_, domainExists, err := readSlabIndexFromRegister(
		s.Ledger,
		address,
		[]byte(domain.Identifier()),
	)
	if err != nil {
		panic(err)
	}

	return domainExists
}

// isV1Account returns true if given account is in account storage format v1
// by checking if any of the domain registers exist.
func (s *Storage) isV1Account(address common.Address) (isV1 bool) {

	// Check if a storage map register exists for any of the domains.
	// Check the most frequently used domains first, such as storage, public, private.
	for _, domain := range common.AllStorageDomains {
		_, domainExists, err := readSlabIndexFromRegister(
			s.Ledger,
			address,
			[]byte(domain.Identifier()),
		)
		if err != nil {
			panic(err)
		}
		if domainExists {
			return true
		}
	}

	return false
}

func (s *Storage) cacheIsV1Account(address common.Address, isV1 bool) {
	if s.cachedV1Accounts == nil {
		s.cachedV1Accounts = map[common.Address]bool{}
	}
	s.cachedV1Accounts[address] = isV1
}

func (s *Storage) cacheDomainStorageMap(
	storageDomainKey interpreter.StorageDomainKey,
	domainStorageMap *interpreter.DomainStorageMap,
) {
	if s.cachedDomainStorageMaps == nil {
		s.cachedDomainStorageMaps = map[interpreter.StorageDomainKey]*interpreter.DomainStorageMap{}
	}

	s.cachedDomainStorageMaps[storageDomainKey] = domainStorageMap
}

func (s *Storage) recordContractUpdate(
	location common.AddressLocation,
	contractValue *interpreter.CompositeValue,
) {
	key := interpreter.NewStorageKey(s.memoryGauge, location.Address, location.Name)

	// NOTE: do NOT delete the map entry,
	// otherwise the removal write is lost

	if s.contractUpdates == nil {
		s.contractUpdates = &orderedmap.OrderedMap[interpreter.StorageKey, *interpreter.CompositeValue]{}
	}
	s.contractUpdates.Set(key, contractValue)
}

func (s *Storage) contractUpdateRecorded(
	location common.AddressLocation,
) bool {
	if s.contractUpdates == nil {
		return false
	}

	key := interpreter.NewStorageKey(s.memoryGauge, location.Address, location.Name)
	return s.contractUpdates.Contains(key)
}

type ContractUpdate struct {
	ContractValue *interpreter.CompositeValue
	Key           interpreter.StorageKey
}

func SortContractUpdates(updates []ContractUpdate) {
	sort.Slice(updates, func(i, j int) bool {
		a := updates[i].Key
		b := updates[j].Key
		return a.IsLess(b)
	})
}

// commitContractUpdates writes the contract updates to storage.
// The contract updates were delayed so they are not observable during execution.
func (s *Storage) commitContractUpdates(inter *interpreter.Interpreter) {
	if s.contractUpdates == nil {
		return
	}

	for pair := s.contractUpdates.Oldest(); pair != nil; pair = pair.Next() {
		s.writeContractUpdate(inter, pair.Key, pair.Value)
	}
}

func (s *Storage) writeContractUpdate(
	inter *interpreter.Interpreter,
	key interpreter.StorageKey,
	contractValue *interpreter.CompositeValue,
) {
	storageMap := s.GetDomainStorageMap(inter, key.Address, common.StorageDomainContract, true)
	// NOTE: pass nil instead of allocating a Value-typed  interface that points to nil
	storageMapKey := interpreter.StringStorageMapKey(key.Key)
	if contractValue == nil {
		storageMap.WriteValue(inter, storageMapKey, nil)
	} else {
		storageMap.WriteValue(inter, storageMapKey, contractValue)
	}
}

// Commit serializes/saves all values in the readCache in storage (through the runtime interface).
func (s *Storage) Commit(inter *interpreter.Interpreter, commitContractUpdates bool) error {
	return s.commit(inter, commitContractUpdates, true)
}

// Deprecated: NondeterministicCommit serializes and commits all values in the deltas storage
// in nondeterministic order.  This function is used when commit ordering isn't
// required (e.g. migration programs).
func (s *Storage) NondeterministicCommit(inter *interpreter.Interpreter, commitContractUpdates bool) error {
	return s.commit(inter, commitContractUpdates, false)
}

func (s *Storage) commit(inter *interpreter.Interpreter, commitContractUpdates bool, deterministic bool) error {

	if commitContractUpdates {
		s.commitContractUpdates(inter)
	}

	err := s.AccountStorageV1.commit()
	if err != nil {
		return err
	}

	if s.Config.StorageFormatV2Enabled {
		err = s.AccountStorageV2.commit()
		if err != nil {
			return err
		}

		err = s.migrateV1AccountsToV2(inter)
		if err != nil {
			return err
		}
	}

	// Commit the underlying slab storage's writes

	slabStorage := s.PersistentSlabStorage

	size := slabStorage.DeltasSizeWithoutTempAddresses()
	if size > 0 {
		inter.ReportComputation(common.ComputationKindEncodeValue, uint(size))
		usage := common.NewBytesMemoryUsage(int(size))
		common.UseMemory(inter, usage)
	}

	deltas := slabStorage.DeltasWithoutTempAddresses()
	common.UseMemory(inter, common.NewAtreeEncodedSlabMemoryUsage(deltas))

	// TODO: report encoding metric for all encoded slabs
	if deterministic {
		return slabStorage.FastCommit(runtime.NumCPU())
	} else {
		return slabStorage.NondeterministicFastCommit(runtime.NumCPU())
	}
}

func (s *Storage) ScheduleV2Migration(address common.Address) {
	s.scheduledV2Migrations = append(s.scheduledV2Migrations, address)
}

func (s *Storage) ScheduleV2MigrationForModifiedAccounts() {
	for address, isV1 := range s.cachedV1Accounts { //nolint:maprange
		if isV1 && s.PersistentSlabStorage.HasUnsavedChanges(atree.Address(address)) {
			s.ScheduleV2Migration(address)
		}
	}
}

func (s *Storage) migrateV1AccountsToV2(inter *interpreter.Interpreter) error {

	if !s.Config.StorageFormatV2Enabled {
		return errors.NewUnexpectedError("cannot migrate to storage format v2, as it is not enabled")
	}

	if len(s.scheduledV2Migrations) == 0 {
		return nil
	}

	// getDomainStorageMap function returns cached domain storage map if it is available
	// before loading domain storage map from storage.
	// This is necessary to migrate uncommitted (new) but cached domain storage map.
	getDomainStorageMap := func(
		ledger atree.Ledger,
		storage atree.SlabStorage,
		address common.Address,
		domain common.StorageDomain,
	) (*interpreter.DomainStorageMap, error) {
		domainStorageKey := interpreter.NewStorageDomainKey(s.memoryGauge, address, domain)

		// Get cached domain storage map if available.
		domainStorageMap := s.cachedDomainStorageMaps[domainStorageKey]

		if domainStorageMap != nil {
			return domainStorageMap, nil
		}

		return getDomainStorageMapFromV1DomainRegister(ledger, storage, address, domain)
	}

	migrator := NewDomainRegisterMigration(
		s.Ledger,
		s.PersistentSlabStorage,
		inter,
		s.memoryGauge,
		getDomainStorageMap,
	)

	// Ensure the scheduled accounts are migrated in a deterministic order

	sort.Slice(
		s.scheduledV2Migrations,
		func(i, j int) bool {
			address1 := s.scheduledV2Migrations[i]
			address2 := s.scheduledV2Migrations[j]
			return address1.Compare(address2) < 0
		},
	)

	for _, address := range s.scheduledV2Migrations {

		accountStorageMap, err := migrator.MigrateAccount(address)
		if err != nil {
			return err
		}

		s.AccountStorageV2.cacheAccountStorageMap(
			address,
			accountStorageMap,
		)

		s.cacheIsV1Account(address, false)
	}

	s.scheduledV2Migrations = nil

	return nil
}

func (s *Storage) CheckHealth() error {

	// Check slab storage health
	rootSlabIDs, err := atree.CheckStorageHealth(s, -1)
	if err != nil {
		return err
	}

	// Find account / non-temporary root slab IDs

	accountRootSlabIDs := make(map[atree.SlabID]struct{}, len(rootSlabIDs))

	// NOTE: map range is safe, as it creates a subset
	for rootSlabID := range rootSlabIDs { //nolint:maprange
		if rootSlabID.HasTempAddress() {
			continue
		}

		accountRootSlabIDs[rootSlabID] = struct{}{}
	}

	// Check that account storage maps and unmigrated domain storage maps
	// match returned root slabs from atree.CheckStorageHealth.

	var storageMapStorageIDs []atree.SlabID

	if s.Config.StorageFormatV2Enabled {
		// Get cached account storage map slab IDs.
		storageMapStorageIDs = append(
			storageMapStorageIDs,
			s.AccountStorageV2.cachedRootSlabIDs()...,
		)
	}

	// Get slab IDs of cached domain storage maps that are in account storage format v1.
	for storageKey, storageMap := range s.cachedDomainStorageMaps { //nolint:maprange
		address := storageKey.Address

		// Only accounts in storage format v1 store domain storage maps
		// directly at the root of the account
		if !s.isV1Account(address) {
			continue
		}

		storageMapStorageIDs = append(
			storageMapStorageIDs,
			storageMap.SlabID(),
		)
	}

	sort.Slice(
		storageMapStorageIDs,
		func(i, j int) bool {
			a := storageMapStorageIDs[i]
			b := storageMapStorageIDs[j]
			return a.Compare(b) < 0
		},
	)

	found := map[atree.SlabID]struct{}{}

	for _, storageMapStorageID := range storageMapStorageIDs {
		if _, ok := accountRootSlabIDs[storageMapStorageID]; !ok {
			return errors.NewUnexpectedError(
				"account storage map (and unmigrated domain storage map) points to non-root slab %s",
				storageMapStorageID,
			)
		}

		found[storageMapStorageID] = struct{}{}
	}

	// Check that all slabs in slab storage
	// are referenced by storables in account storage.
	// If a slab is not referenced, it is garbage.

	if len(accountRootSlabIDs) > len(found) {
		var unreferencedRootSlabIDs []atree.SlabID

		for accountRootSlabID := range accountRootSlabIDs { //nolint:maprange
			if _, ok := found[accountRootSlabID]; ok {
				continue
			}

			unreferencedRootSlabIDs = append(
				unreferencedRootSlabIDs,
				accountRootSlabID,
			)
		}

		sort.Slice(unreferencedRootSlabIDs, func(i, j int) bool {
			a := unreferencedRootSlabIDs[i]
			b := unreferencedRootSlabIDs[j]
			return a.Compare(b) < 0
		})

		return UnreferencedRootSlabsError{
			UnreferencedRootSlabIDs: unreferencedRootSlabIDs,
		}
	}

	return nil
}

type UnreferencedRootSlabsError struct {
	UnreferencedRootSlabIDs []atree.SlabID
}

var _ errors.InternalError = UnreferencedRootSlabsError{}

func (UnreferencedRootSlabsError) IsInternalError() {}

func (e UnreferencedRootSlabsError) Error() string {
	return fmt.Sprintf("slabs not referenced: %s", e.UnreferencedRootSlabIDs)
}
