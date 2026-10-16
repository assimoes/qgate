import pytest

from app.views import discount, greet


def test_greet():
    assert greet("Ada") == "Hi Ada"


def test_discount_member():
    assert discount(200, True) == 180


@pytest.fixture
def db():
    raise RuntimeError("no database")


def test_save(db):
    assert db


@pytest.mark.skip(reason="slow")
def test_slow():
    pass
