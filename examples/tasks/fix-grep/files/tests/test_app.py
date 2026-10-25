import unittest

from app import slugify


class TestSlugify(unittest.TestCase):
    def test_basic(self):
        self.assertEqual(slugify("Hello World"), "hello-world")

    def test_trims_dashes(self):
        self.assertEqual(slugify("--Hello--"), "hello")

    def test_symbols(self):
        self.assertEqual(slugify("A & B!"), "a-b")

    def test_already_clean(self):
        self.assertEqual(slugify("ok-2026"), "ok-2026")


if __name__ == "__main__":
    unittest.main()
