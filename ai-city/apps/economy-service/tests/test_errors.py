import pytest
from economy_service.errors import (
    BTInvalidError,
    CreatorRequiredError,
    EconomyError,
    PriceInvalidError,
    PurchaseDuplicateError,
    SelfPurchaseError,
    TemplateNotFoundError,
    TemplateTakenDownError,
    YamlInvalidError,
)


@pytest.mark.parametrize(
    ("exception_type", "code", "http_status", "message"),
    [
        (CreatorRequiredError, "R_027", 403, "creator role required"),
        (TemplateNotFoundError, "R_028", 404, "template not found"),
        (TemplateTakenDownError, "R_029", 410, "template taken down"),
        (PriceInvalidError, "R_030", 400, "price must be >= 10"),
        (BTInvalidError, "R_031", 400, "BT skeleton invalid"),
        (YamlInvalidError, "R_032", 400, "YAML content invalid"),
        (SelfPurchaseError, "R_033", 403, "creator cannot purchase own template"),
        (PurchaseDuplicateError, "R_034", 409, "purchase duplicate (idempotency)"),
    ],
)
def test_marketplace_error_contract(exception_type, code, http_status, message):
    assert issubclass(exception_type, EconomyError)
    assert exception_type.code == code
    assert exception_type.http_status == http_status

    error = exception_type()

    assert error.code == code
    assert error.http_status == http_status
    assert error.msg == message
