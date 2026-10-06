# apps/economy-service/src/economy_service/errors.py
"""Custom exceptions for economy-service."""
from __future__ import annotations


class EconomyError(Exception):
    code: str = "R_018"
    http_status: int = 400

    def __init__(self, msg: str):
        self.msg = msg
        super().__init__(msg)


class InsufficientBalance(EconomyError):  # noqa: N818 (named per spec)
    code = "R_022"
    http_status = 402


class TransferSelf(EconomyError):  # noqa: N818 (named per spec)
    code = "R_023"
    http_status = 400
