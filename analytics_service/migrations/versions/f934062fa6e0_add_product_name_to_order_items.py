"""add_product_name_to_order_items

Revision ID: f934062fa6e0
Revises: c43980076726
Create Date: 2025-12-16 19:43:04.781352

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'f934062fa6e0'
down_revision: Union[str, Sequence[str], None] = 'c43980076726'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column('raw_order_items', sa.Column('product_name', sa.String(length=255), nullable=True))

def downgrade() -> None:
    op.drop_column('raw_order_items', 'product_name')
