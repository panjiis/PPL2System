"""create_raw_employees_table

Revision ID: c43980076726
Revises: f29d133cc07c
Create Date: 2025-12-16 17:54:03.618288

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'c43980076726'
down_revision: Union[str, Sequence[str], None] = 'f29d133cc07c'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # --- COPY DARI SINI ---
    op.create_table(
        'raw_employees',
        sa.Column('employee_id', sa.BigInteger(), autoincrement=False, nullable=False),
        sa.Column('name', sa.String(length=255), nullable=False),
        sa.Column('role', sa.String(length=50), nullable=True),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('NOW()'), nullable=True),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('NOW()'), nullable=True),
        sa.PrimaryKeyConstraint('employee_id')
    )
    # ----------------------


def downgrade() -> None:
    # --- COPY DARI SINI ---
    op.drop_table('raw_employees')
    # ----------------------
