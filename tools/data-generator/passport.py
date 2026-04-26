"""Synthetic Russian passport PDF generator using fpdf2.

Generates a plausible-looking passport data page with:
  - Series (4 digits): first 2 = region code, last 2 = year
  - Number (6 digits): sequential number
  - Full name (fake, in Russian transliteration)
  - Date of birth
  - Place of birth (fictional Russian city)
  - Issued by (fictional department name + code)
  - Issue date
  - Gender

All data is fully synthetic. No real personal information is used.
Seed 42 is the default for reproducibility.
"""

from __future__ import annotations

import random
from dataclasses import dataclass
from pathlib import Path

from faker import Faker
from fpdf import FPDF

# Russian locale faker for realistic-looking names and cities.
_faker_ru = Faker("ru_RU")

# System font paths with Cyrillic support (tried in order).
_UNICODE_FONT_CANDIDATES = [
    "/Library/Fonts/Arial Unicode.ttf",
    "/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
]

_UNICODE_FONT_PATH: str | None = None
for _candidate in _UNICODE_FONT_CANDIDATES:
    if Path(_candidate).exists():
        _UNICODE_FONT_PATH = _candidate
        break

_CITIES = [
    "Москва", "Санкт-Петербург", "Новосибирск", "Екатеринбург", "Казань",
    "Нижний Новгород", "Челябинск", "Самара", "Уфа", "Ростов-на-Дону",
    "Краснодар", "Омск", "Воронеж", "Пермь", "Волгоград", "Саратов",
    "Тюмень", "Тольятти", "Ижевск", "Барнаул",
]

_DEPARTMENTS = [
    "ОУФМС России по {city} р-ну",
    "МФЦ {city}",
    "УМВД России по г. {city}",
    "Отдел по вопросам миграции ОМВД России по {city}",
]


@dataclass
class PassportData:
    series: str          # "ХXХX" — 4 digits
    number: str          # 6 digits
    last_name: str
    first_name: str
    middle_name: str
    dob: str             # DD.MM.YYYY
    place_of_birth: str
    gender: str          # М / Ж
    issue_date: str      # DD.MM.YYYY
    issued_by: str
    division_code: str   # XXX-XXX


def _fmt_date(dt) -> str:
    return dt.strftime("%d.%m.%Y")


def generate_passport_data(rng: random.Random | None = None) -> PassportData:
    """Generate a PassportData record with synthetic values."""
    rng = rng or random.Random(42)
    Faker.seed(rng.randint(0, 2**31))

    fake = _faker_ru

    gender_flag = rng.choice(["male", "female"])
    if gender_flag == "male":
        first = fake.first_name_male()
        last = fake.last_name_male()
        middle = fake.middle_name_male()
        gender = "М"
    else:
        first = fake.first_name_female()
        last = fake.last_name_female()
        middle = fake.middle_name_female()
        gender = "Ж"

    region = rng.randint(1, 99)
    # Passport series year: 97–99 (1990s) or 00–23 (2000s–2020s)
    year_2digit = rng.choice(list(range(97, 100)) + list(range(0, 24)))
    series = f"{region:02d}{year_2digit:02d}"
    number = f"{rng.randint(100000, 999999)}"

    dob = fake.date_of_birth(minimum_age=18, maximum_age=80)
    place_of_birth = rng.choice(_CITIES)

    issue_age = rng.choice([20, 45])  # Russian passports renewed at 20 and 45
    from datetime import date, timedelta
    issue_year = dob.year + issue_age + rng.randint(0, 2)
    issue_date = date(min(issue_year, date.today().year), rng.randint(1, 12), rng.randint(1, 28))

    city = rng.choice(_CITIES)
    dept_template = rng.choice(_DEPARTMENTS)
    issued_by = dept_template.format(city=city)

    div_region = rng.randint(1, 99)
    div_num = rng.randint(1, 999)
    division_code = f"{div_region:03d}-{div_num:03d}"

    return PassportData(
        series=series,
        number=number,
        last_name=last,
        first_name=first,
        middle_name=middle,
        dob=_fmt_date(dob),
        place_of_birth=place_of_birth,
        gender=gender,
        issue_date=_fmt_date(issue_date),
        issued_by=issued_by,
        division_code=division_code,
    )


class PassportPDF(FPDF):
    """FPDF subclass that renders a synthetic Russian passport data page."""

    _FONT_NAME = "UniFont"

    def _setup_font(self) -> None:
        """Register a Unicode TTF font that supports Cyrillic."""
        if _UNICODE_FONT_PATH is None:
            raise RuntimeError(
                "No Unicode TTF font found on this system. "
                "Install Arial Unicode or DejaVu fonts."
            )
        self.add_font(self._FONT_NAME, style="", fname=_UNICODE_FONT_PATH)
        self.add_font(self._FONT_NAME, style="B", fname=_UNICODE_FONT_PATH)
        self.add_font(self._FONT_NAME, style="I", fname=_UNICODE_FONT_PATH)

    def _sf(self, size: int, bold: bool = False, italic: bool = False) -> None:
        style = ("B" if bold else "") + ("I" if italic else "")
        self.set_font(self._FONT_NAME, style=style, size=size)

    def _row(self, label: str, value: str, y: float) -> None:
        self.set_xy(10, y)
        self._sf(8, bold=True)
        self.set_text_color(100, 100, 100)
        self.cell(60, 6, label, border=0)
        self._sf(10)
        self.set_text_color(0, 0, 0)
        self.cell(130, 6, value, border=0)

    def render(self, data: PassportData) -> None:
        self._setup_font()
        self.add_page()

        # Header bar
        self.set_fill_color(0, 51, 102)
        self.rect(0, 0, 210, 20, style="F")
        self.set_xy(0, 4)
        self._sf(13, bold=True)
        self.set_text_color(255, 255, 255)
        self.cell(210, 12, "ПАСПОРТ ГРАЖДАНИНА РОССИЙСКОЙ ФЕДЕРАЦИИ", align="C")

        # Subheader (Latin only — safe with any font)
        self.set_xy(0, 20)
        self._sf(9)
        self.set_text_color(50, 50, 50)
        self.cell(210, 8, "PASSPORT OF A CITIZEN OF THE RUSSIAN FEDERATION", align="C")

        # Series / Number box
        self.set_fill_color(240, 240, 240)
        self.rect(10, 32, 90, 14, style="F")
        self.set_xy(10, 33)
        self._sf(8, bold=True)
        self.set_text_color(80, 80, 80)
        self.cell(45, 5, "Серия / Series", border=0)
        self.cell(45, 5, "Номер / Number", border=0)
        self.set_xy(10, 39)
        self._sf(14, bold=True)
        self.set_text_color(0, 0, 0)
        self.cell(45, 7, data.series, border=0)
        self.cell(45, 7, data.number, border=0)

        # Separator line
        self.set_draw_color(180, 180, 180)
        self.line(10, 50, 200, 50)

        # Personal data rows
        rows = [
            ("Фамилия / Surname", data.last_name),
            ("Имя / Given name", data.first_name),
            ("Отчество / Patronymic", data.middle_name),
            ("Пол / Sex", data.gender),
            ("Дата рождения / Date of birth", data.dob),
            ("Место рождения / Place of birth", data.place_of_birth),
        ]
        y = 54
        for label, value in rows:
            self._row(label, value, y)
            y += 10
            self.set_draw_color(220, 220, 220)
            self.line(10, y - 1, 200, y - 1)

        # Issue section header
        self.set_fill_color(240, 240, 240)
        self.rect(10, y + 2, 190, 8, style="F")
        self.set_xy(10, y + 3)
        self._sf(8, bold=True)
        self.set_text_color(80, 80, 80)
        self.cell(190, 6, "СВЕДЕНИЯ О ВЫДАЧЕ / ISSUANCE DETAILS", border=0, align="C")
        y += 14

        issue_rows = [
            ("Дата выдачи / Issue date", data.issue_date),
            ("Кем выдан / Issued by", data.issued_by),
            ("Код подразделения / Division code", data.division_code),
        ]
        for label, value in issue_rows:
            self._row(label, value, y)
            y += 10
            self.set_draw_color(220, 220, 220)
            self.line(10, y - 1, 200, y - 1)

        # Synthetic watermark
        self.set_xy(10, y + 6)
        self._sf(7, italic=True)
        self.set_text_color(180, 180, 180)
        self.cell(
            190, 6,
            "SYNTHETIC DOCUMENT - NOT VALID - GENERATED FOR TESTING PURPOSES ONLY",
            align="C",
        )

        # Footer
        self.set_fill_color(0, 51, 102)
        self.rect(0, 282, 210, 15, style="F")
        self.set_xy(0, 285)
        self._sf(7)
        self.set_text_color(200, 200, 200)
        self.cell(210, 8, "AIbank Test Data Generator - Fully Synthetic", align="C")


def generate_passport_pdf(
    output_path: str | Path,
    rng: random.Random | None = None,
) -> PassportData:
    """
    Generate a synthetic passport PDF and return the PassportData used.

    Args:
        output_path: Path to write the PDF file.
        rng: Optional random.Random instance for reproducibility.

    Returns:
        PassportData with all generated field values.
    """
    rng = rng or random.Random(42)
    data = generate_passport_data(rng)

    pdf = PassportPDF(orientation="P", unit="mm", format="A4")
    pdf.set_auto_page_break(auto=False)
    pdf.set_margins(0, 0, 0)
    pdf.render(data)

    output_path = Path(output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    pdf.output(str(output_path))

    return data


if __name__ == "__main__":
    import sys
    out = sys.argv[1] if len(sys.argv) > 1 else "/tmp/synthetic_passport.pdf"
    data = generate_passport_pdf(out)
    print(f"Generated: {out}")
    print(f"  Name:   {data.last_name} {data.first_name} {data.middle_name}")
    print(f"  Series: {data.series}  Number: {data.number}")
    print(f"  DOB:    {data.dob}")
