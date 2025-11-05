from decimal import Decimal, ROUND_HALF_UP

def to_string(d: Decimal | None) -> str:
    if d is None:
        return "0.00"
    return str(d.quantize(Decimal('0.01'), rounding=ROUND_HALF_UP))

def to_percent_string(d: Decimal | None) -> str:
    if d is None:
        return "0.00"
    return str(d.quantize(Decimal('0.01'), rounding=ROUND_HALF_UP))
