# apps/economy-service/src/economy_service/errors.py
"""Custom exceptions for economy-service."""
from __future__ import annotations


class EconomyError(Exception):
    code: str = "R_018"
    http_status: int = 400
    default_msg = "economy error"

    def __init__(self, msg: str | None = None):
        self.msg = msg or self.default_msg
        super().__init__(msg)


class InsufficientBalance(EconomyError):  # noqa: N818 (named per spec)
    code = "R_022"
    http_status = 402


class TransferSelf(EconomyError):  # noqa: N818 (named per spec)
    code = "R_023"
    http_status = 400


class WalletNotFound(EconomyError):  # noqa: N818 (named per spec)
    code = "R_018"
    http_status = 404


class ProductOutOfStockError(EconomyError):
    code = "R_024"
    http_status = 409


class ProductNotFoundError(EconomyError):
    code = "R_025"
    http_status = 404


class CreatorRequiredError(EconomyError):
    code = "R_027"
    http_status = 403
    default_msg = "creator role required"


class TemplateNotFoundError(EconomyError):
    code = "R_028"
    http_status = 404
    default_msg = "template not found"


class TemplateTakenDownError(EconomyError):
    code = "R_029"
    http_status = 410
    default_msg = "template taken down"


class PriceInvalidError(EconomyError):
    code = "R_030"
    http_status = 400
    default_msg = "price must be >= 10"


class BTInvalidError(EconomyError):
    code = "R_031"
    http_status = 400
    default_msg = "BT skeleton invalid"


class YamlInvalidError(EconomyError):
    code = "R_032"
    http_status = 400
    default_msg = "YAML content invalid"


class SelfPurchaseError(EconomyError):
    code = "R_033"
    http_status = 403
    default_msg = "creator cannot purchase own template"


class PurchaseDuplicateError(EconomyError):
    code = "R_034"
    http_status = 409
    default_msg = "purchase duplicate (idempotency)"


class CrossCityValidationError(EconomyError):
    code = "CROSS_CITY_VALIDATION_FAILED"
    http_status = 400
    default_msg = "cross-city transfer validation failed"


class CrossCityUnsupportedCurrencyError(EconomyError):
    code = "CROSS_CITY_UNSUPPORTED_CURRENCY"
    http_status = 400
    default_msg = "cross-city currency not enabled"


class CrossCityAmountOutOfRangeError(EconomyError):
    code = "CROSS_CITY_AMOUNT_OUT_OF_RANGE"
    http_status = 400
    default_msg = "cross-city amount out of range"


class CrossCitySameCityError(EconomyError):
    code = "CROSS_CITY_SAME_CITY"
    http_status = 400
    default_msg = "source and destination city must differ"


class CrossCityIdempotencyConflictError(EconomyError):
    code = "CROSS_CITY_IDEMPOTENCY_CONFLICT"
    http_status = 409
    default_msg = "idempotency key reused with different transfer payload"


class CrossCityTransferNotFoundError(EconomyError):
    code = "CROSS_CITY_SOURCE_NOT_FOUND"
    http_status = 404
    default_msg = "cross-city transfer not found"


class CrossCityExpiredError(EconomyError):
    code = "CROSS_CITY_EXPIRED"
    http_status = 410
    default_msg = "cross-city transfer expired"


class CrossCityInvalidStateError(EconomyError):
    code = "CROSS_CITY_INVALID_STATE"
    http_status = 409
    default_msg = "cross-city transfer state transition is not allowed"
