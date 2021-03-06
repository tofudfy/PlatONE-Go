package vm

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/PlatONEnetwork/PlatONE-Go/common"
	"github.com/PlatONEnetwork/PlatONE-Go/params"
	"github.com/PlatONEnetwork/PlatONE-Go/rlp"
)

var (
	gasContractNameKey              = generateStateKey("GasContractName")
	isProduceEmptyBlockKey          = generateStateKey("IsProduceEmptyBlock")
	txGasLimitKey                   = generateStateKey("TxGasLimit")
	blockGasLimitKey                = generateStateKey("BlockGasLimit")
	isCheckContractDeployPermission = generateStateKey("isCheckContractDeployPermission")
	isApproveDeployedContractKey    = generateStateKey("IsApproveDeployedContract")
	isTxUseGasKey                   = generateStateKey("IsTxUseGas")

	vrfParamsKey          = generateStateKey("VRFParamsKey")
	isBlockUseTrieHashKey = generateStateKey("IsBlockUseTrieHash")
)

const (
	paramTrue  uint32 = 1
	paramFalse uint32 = 0
)

const (
	isCheckContractDeployPermissionDefault = paramFalse
	isTxUseGasDefault                      = paramFalse
	isApproveDeployedContractDefault       = paramFalse
	isProduceEmptyBlockDefault             = paramFalse
	gasContractNameDefault                 = ""
	isBlockUseTrieHashDefault              = paramTrue
)

const (
	TxGasLimitMinValue        uint64 = 12771596 * 100 // 12771596 大致相当于 0.012772s
	TxGasLimitMaxValue        uint64 = 2e9            // 相当于 2s
	txGasLimitDefaultValue    uint64 = 1.5e9          // 相当于 1.5s
	BlockGasLimitMinValue     uint64 = 12771596 * 100 // 12771596 大致相当于 0.012772s
	BlockGasLimitMaxValue     uint64 = 2e10           // 相当于 20s
	blockGasLimitDefaultValue uint64 = 1e10           // 相当于 10s
	failFlag                         = -1
	sucFlag                          = 0
)
const (
	doParamSetSuccess     CodeType = 0
	callerHasNoPermission CodeType = 1
	encodeFailure         CodeType = 2
	paramInvalid          CodeType = 3
	contractNameNotExists CodeType = 4
)

type ParamManager struct {
	stateDB      StateDB
	contractAddr *common.Address
	caller       common.Address
	blockNumber  *big.Int
}

func encode(i interface{}) ([]byte, error) {
	return rlp.EncodeToBytes(i)
}

func (u *ParamManager) RequiredGas(input []byte) uint64 {
	if common.IsBytesEmpty(input) {
		return 0
	}
	return params.ParamManagerGas
}

func (u *ParamManager) Run(input []byte) ([]byte, error) {
	fnName, ret, err := execSC(input, u.AllExportFns())
	if err != nil {
		if fnName == "" {
			fnName = "Notify"
		}
		u.emitNotifyEventInParam(fnName, operateFail, err.Error())
	}
	return ret, nil
}

func (u *ParamManager) setState(key, value []byte) {
	u.stateDB.SetState(*u.contractAddr, key, value)
}

func (u *ParamManager) getState(key []byte) []byte {
	return u.stateDB.GetState(*u.contractAddr, key)
}

func (u *ParamManager) setGasContractName(contractName string) (int32, error) {
	if b, _ := checkNameFormat(contractName); !b {
		u.emitNotifyEventInParam("GasContractName", paramInvalid, fmt.Sprintf("param is invalid."))
		return failFlag, errParamInvalid
	}
	res, err := getRegisterStatusByName(u.stateDB, contractName)
	if !res {
		u.emitNotifyEventInParam("GasContractName", contractNameNotExists, fmt.Sprintf("contract does not exsits."))
		return failFlag, errContactNameNotExist
	}
	ret, err := u.doParamSet(gasContractNameKey, contractName)

	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("GasContractName", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("GasContractName", encodeFailure, fmt.Sprintf("%v failed to encode.", gasContractNameKey))
			return failFlag, err
		}

	}
	u.emitNotifyEventInParam("GasContractName", doParamSetSuccess, fmt.Sprintf("param set successful."))
	return ret, err
}

//pass
func (u *ParamManager) getGasContractName() (string, error) {
	contractName := gasContractNameDefault
	err := u.getParam(gasContractNameKey, &contractName)
	if err != nil && err != errEmptyValue {
		return "", err
	}
	return contractName, nil
}

//pass
func (u *ParamManager) setIsProduceEmptyBlock(isProduceEmptyBlock uint32) (int32, error) {
	if isProduceEmptyBlock/2 != 0 {
		u.emitNotifyEventInParam("IsProduceEmptyBlock", paramInvalid, fmt.Sprintf("param is invalid."))
		return failFlag, errParamInvalid
	}
	ret, err := u.doParamSet(isProduceEmptyBlockKey, isProduceEmptyBlock)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("IsProduceEmptyBlock", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("IsProduceEmptyBlock", encodeFailure, fmt.Sprintf("%v failed to encode.", isProduceEmptyBlockKey))
			return failFlag, err
		}
	}
	u.emitNotifyEventInParam("IsProduceEmptyBlock", doParamSetSuccess, fmt.Sprintf("param set successful."))
	return ret, err
}

func (u *ParamManager) getIsProduceEmptyBlock() (uint32, error) {
	isProduceEmptyBlock := isProduceEmptyBlockDefault
	err := u.getParam(isProduceEmptyBlockKey, &isProduceEmptyBlock)
	if err != nil && err != errEmptyValue {
		return isProduceEmptyBlockDefault, err
	}
	return isProduceEmptyBlock, nil
}

func (u *ParamManager) setTxGasLimit(txGasLimit uint64) (int32, error) {
	if txGasLimit < TxGasLimitMinValue || txGasLimit > TxGasLimitMaxValue {
		u.emitNotifyEventInParam("TxGasLimit", paramInvalid, fmt.Sprintf("param is invalid."))

		return failFlag, errParamInvalid
	}
	// 获取区块 gas limit，其值应大于或等于每笔交易 gas limit 参数的值
	blockGasLimit := blockGasLimitDefaultValue
	err := u.getParam(blockGasLimitKey, &blockGasLimit)
	if err != nil && err != errEmptyValue {
		return failFlag, err
	}
	if txGasLimit > blockGasLimit {
		u.emitNotifyEventInParam("TxGasLimit", paramInvalid, fmt.Sprintf("setting value is larger than block gas limit"))
		return failFlag, errParamInvalid
	}

	ret, err := u.doParamSet(txGasLimitKey, txGasLimit)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("TxGasLimit", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("TxGasLimit", encodeFailure, fmt.Sprintf("%v failed to encode.", txGasLimitKey))
			return failFlag, err
		}
	}
	u.emitNotifyEventInParam("TxGasLimit", doParamSetSuccess, fmt.Sprintf("param set successful."))

	return ret, err
}

func (u *ParamManager) getTxGasLimit() (uint64, error) {
	txGasLimit := txGasLimitDefaultValue
	err := u.getParam(txGasLimitKey, &txGasLimit)
	if err != nil && err != errEmptyValue {
		return txGasLimitDefaultValue, err
	}
	return txGasLimit, nil
}

func (u *ParamManager) setBlockGasLimit(blockGasLimit uint64) (int32, error) {
	if blockGasLimit < BlockGasLimitMinValue || blockGasLimit > BlockGasLimitMaxValue {
		u.emitNotifyEventInParam("BlockGasLimit", paramInvalid, fmt.Sprintf("param is invalid."))
		return failFlag, errParamInvalid
	}
	key := txGasLimitKey
	txGasLimit := txGasLimitDefaultValue
	err := u.getParam(key, &txGasLimit)
	if err != nil && err != errEmptyValue {
		return failFlag, err
	}
	if txGasLimit > blockGasLimit {
		u.emitNotifyEventInParam("BlockGasLimit", paramInvalid, fmt.Sprintf("setting value is smaller than tx gas limit"))
		return failFlag, nil
	}

	ret, err := u.doParamSet(blockGasLimitKey, blockGasLimit)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("BlockGasLimit", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("BlockGasLimit", encodeFailure, fmt.Sprintf("%v failed to encode.", blockGasLimitKey))
			return failFlag, err
		}
	}
	u.emitNotifyEventInParam("BlockGasLimit", doParamSetSuccess, fmt.Sprintf("param set successful."))

	return ret, err
}

// 获取区块 gaslimit
func (u *ParamManager) getBlockGasLimit() (uint64, error) {
	var b = blockGasLimitDefaultValue
	if err := u.getParam(blockGasLimitKey, &b); err != nil && err != errEmptyValue {
		return blockGasLimitDefaultValue, err
	}
	return b, nil
}

// 设置是否检查合约部署权限
// 0: 不检查合约部署权限，允许任意用户部署合约  1: 检查合约部署权限，用户具有相应权限才可以部署合约
// 默认为0，不检查合约部署权限，即允许任意用户部署合约
func (u *ParamManager) setCheckContractDeployPermission(permission uint32) (int32, error) {
	if permission/2 != 0 {
		u.emitNotifyEventInParam("IsCheckContractDeployPermission", paramInvalid, fmt.Sprintf("param is invalid."))

		return failFlag, errParamInvalid
	}
	ret, err := u.doParamSet(isCheckContractDeployPermission, permission)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("IsCheckContractDeployPermission", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("IsCheckContractDeployPermission", encodeFailure, fmt.Sprintf("%v failed to encode.", isCheckContractDeployPermission))
			return failFlag, err
		}
	}
	u.emitNotifyEventInParam("IsCheckContractDeployPermission", doParamSetSuccess, fmt.Sprintf("param set successful."))

	return ret, err
}

// 获取是否是否检查合约部署权限
// 0: 不检查合约部署权限，允许任意用户部署合约  1: 检查合约部署权限，用户具有相应权限才可以部署合约
// 默认为0，不检查合约部署权限，即允许任意用户部署合约
func (u *ParamManager) getCheckContractDeployPermission() (uint32, error) {
	var b = isCheckContractDeployPermissionDefault
	if err := u.getParam(isCheckContractDeployPermission, &b); err != nil && err != errEmptyValue {
		return isCheckContractDeployPermissionDefault, err
	}
	return b, nil
}

// 设置是否审核已部署的合约
// @isApproveDeployedContract:
// 1: 审核已部署的合约  0: 不审核已部署的合约
func (u *ParamManager) setIsApproveDeployedContract(isApproveDeployedContract uint32) (int32, error) {
	if isApproveDeployedContract/2 != 0 {
		u.emitNotifyEventInParam("IsApproveDeployedContract", paramInvalid, fmt.Sprintf("param is invalid."))

		return failFlag, errParamInvalid
	}
	ret, err := u.doParamSet(isApproveDeployedContractKey, isApproveDeployedContract)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("IsApproveDeployedContract", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("IsApproveDeployedContract", encodeFailure, fmt.Sprintf("%v failed to encode.", isApproveDeployedContractKey))
			return failFlag, err
		}
	}
	u.emitNotifyEventInParam("IsApproveDeployedContract", doParamSetSuccess, fmt.Sprintf("param set successful."))

	return ret, err
}

// 获取是否审核已部署的合约的标志
func (u *ParamManager) getIsApproveDeployedContract() (uint32, error) {
	var b = isApproveDeployedContractDefault
	if err := u.getParam(isApproveDeployedContractKey, &b); err != nil && err != errEmptyValue {
		return isApproveDeployedContractDefault, err
	}
	return b, nil
}

// 本参数根据最新的讨论（2019.03.06之前）不再需要，即交易需要消耗gas。但是计费相关如消耗特定合约代币的参数由 setGasContractName 进行设置
// 设置交易是否消耗 gas
// @isTxUseGas:
// 1:  交易消耗 gas  0: 交易不消耗 gas
func (u *ParamManager) setIsTxUseGas(isTxUseGas uint64) (int32, error) {
	if isTxUseGas/2 != 0 {
		u.emitNotifyEventInParam("IsTxUseGas", paramInvalid, fmt.Sprintf("param is invalid."))
		return failFlag, errParamInvalid
	}
	ret, err := u.doParamSet(isTxUseGasKey, isTxUseGas)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("IsTxUseGas", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("IsTxUseGas", encodeFailure, fmt.Sprintf("%v failed to encode.", isTxUseGasKey))
			return failFlag, err
		}
	}
	u.emitNotifyEventInParam("IsTxUseGas", doParamSetSuccess, fmt.Sprintf("param set successful."))

	return ret, err
}

func (u *ParamManager) checkVRFParams(params common.VRFParams) error {
	if params.ValidatorCount < 1 {
		return errValidatorCountInvalid
	}
	return nil
}

func (u *ParamManager) setVRFParams(params common.VRFParams) (int32, error) {
	if err := u.checkVRFParams(params); err != nil {
		u.emitNotifyEventInParam("SetVRFParams", paramInvalid, fmt.Sprintf("param is invalid."))
		return 0, err
	}
	_, err := u.doParamSet(vrfParamsKey, params)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("SetVRFParams", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return 0, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("SetVRFParams", encodeFailure, fmt.Sprintf("%v failed to encode.", isTxUseGasKey))
			return 0, err
		}
	}
	u.emitNotifyEventInParam("SetVRFParams", doParamSetSuccess, fmt.Sprintf("param set successful."))
	return 0, nil
}

func (u *ParamManager) getVRFParams() (common.VRFParams, error) {
	b := common.VRFParams{}
	if err := u.getParam(vrfParamsKey, &b); err != nil && err != errEmptyValue {
		return b, err
	}
	return b, nil
}

func (u *ParamManager) getVRFParamsWrapper() (string, error) {
	vrf, err := u.getVRFParams()
	if err != nil {
		return "", nil
	}

	b, err := json.Marshal(vrf)
	if err != nil {
		return "", nil
	}
	return string(b), nil
}

// 获取交易是否消耗 gas
func (u *ParamManager) getIsTxUseGas() (uint32, error) {
	var isUseGas = isTxUseGasDefault
	if err := u.getParam(isTxUseGasKey, &isUseGas); err != nil && err != errEmptyValue {
		return isTxUseGasDefault, err
	}
	return isUseGas, nil
}

// 1:  header 使用trie hash  // 0:
func (u *ParamManager) setIsBlockUseTrieHash(isBlockUseTrieHash uint64) (int32, error) {
	if isBlockUseTrieHash/2 != 0 {
		u.emitNotifyEventInParam("IsBlockUseTrieHash", paramInvalid, fmt.Sprintf("param is invalid."))
		return failFlag, errParamInvalid
	}
	ret, err := u.doParamSet(isBlockUseTrieHashKey, isBlockUseTrieHash)
	if err != nil {
		switch err {
		case errNoPermission:
			u.emitNotifyEventInParam("IsBlockUseTrieHash", callerHasNoPermission, fmt.Sprintf("%s has no permission to adjust param.", u.caller.String()))
			return failFlag, err

		case errEncodeFailure:
			u.emitNotifyEventInParam("IsBlockUseTrieHash", encodeFailure, fmt.Sprintf("%v failed to encode.", isTxUseGasKey))
			return failFlag, err
		}
	}
	u.emitNotifyEventInParam("IsBlockUseTrieHash", doParamSetSuccess, fmt.Sprintf("param set successful."))

	return ret, err
}

// 获取header是否使用trie hash
func (u *ParamManager) getIsBlockUseTrieHash() (uint32, error) {
	var isBlockUseTrieHash = isBlockUseTrieHashDefault
	if err := u.getParam(isBlockUseTrieHashKey, &isBlockUseTrieHash); err != nil && err != errEmptyValue {
		return isBlockUseTrieHashDefault, err
	}
	return isBlockUseTrieHash, nil
}

func (u *ParamManager) doParamSet(key []byte, value interface{}) (int32, error) {
	if !hasParamOpPermission(u.stateDB, u.caller) {
		return failFlag, errNoPermission
	}
	if err := u.setParam(key, value); err != nil {
		return failFlag, errEncodeFailure
	}
	return sucFlag, nil
}

func (u *ParamManager) setParam(key []byte, val interface{}) error {
	value, err := rlp.EncodeToBytes(val)
	if err != nil {
		return err
	}
	u.setState(key, value)
	return nil
}

func (u *ParamManager) getParam(key []byte, val interface{}) error {
	value := u.getState(key)
	if len(value) == 0 {
		return errEmptyValue
	}
	if err := rlp.DecodeBytes(value, val); err != nil {
		return err
	}
	return nil
}

func (u *ParamManager) emitNotifyEventInParam(topic string, code CodeType, msg string) {
	emitEvent(*u.contractAddr, u.stateDB, u.blockNumber.Uint64(), topic, code, msg)
}

//for access control
func (u *ParamManager) AllExportFns() SCExportFns {
	return SCExportFns{
		"setGasContractName":               u.setGasContractName,
		"getGasContractName":               u.getGasContractName,
		"setIsProduceEmptyBlock":           u.setIsProduceEmptyBlock,
		"getIsProduceEmptyBlock":           u.getIsProduceEmptyBlock,
		"setTxGasLimit":                    u.setTxGasLimit,
		"getTxGasLimit":                    u.getTxGasLimit,
		"setBlockGasLimit":                 u.setBlockGasLimit,
		"getBlockGasLimit":                 u.getBlockGasLimit,
		"setCheckContractDeployPermission": u.setCheckContractDeployPermission,
		"getCheckContractDeployPermission": u.getCheckContractDeployPermission,
		"setIsApproveDeployedContract":     u.setIsApproveDeployedContract,
		"getIsApproveDeployedContract":     u.getIsApproveDeployedContract,
		"setIsTxUseGas":                    u.setIsTxUseGas,
		"getIsTxUseGas":                    u.getIsTxUseGas,
		"setVRFParams":                     u.setVRFParams,
		"getVRFParams":                     u.getVRFParamsWrapper,
		"setIsBlockUseTrieHash":            u.setIsBlockUseTrieHash,
		"getIsBlockUseTrieHash":            u.getIsBlockUseTrieHash,
	}
}
