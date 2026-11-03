document.addEventListener("keydown", (event) => {
	if (event.key !== "Escape") {
		return;
	}

	const close = document.querySelector("[data-close]");
	if (close) {
		window.location.href = close.href;
	}
});

const selectedDay = document.querySelector(".day.is-selected");
if (selectedDay) {
	selectedDay.scrollIntoView({ block: "start" });
}

const field = document.querySelector('[aria-invalid="true"]') ?? document.getElementById("content") ?? document.getElementById("feast");
if (field && document.activeElement !== field) {
	field.focus({ preventScroll: true });
}
if (field instanceof HTMLTextAreaElement || (field instanceof HTMLInputElement && field.type === "text")) {
	field.setSelectionRange(field.value.length, field.value.length);
}

const form = document.querySelector(".panel-form");
if (form) {
	const motion = window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth";

	form.addEventListener("change", (event) => {
		const day = form.querySelector('input[name="day"]:checked')?.value;
		const time = form.querySelector('select[name="time"]').value;

		for (const section of document.querySelectorAll(".day")) {
			const selected = section.id === "d-" + day;
			section.classList.toggle("is-selected", selected);
			for (const mass of section.querySelectorAll(".mass")) {
				mass.classList.toggle("is-selected", selected && mass.dataset.time === time);
			}
		}

		if (event.target.name === "day") {
			document.getElementById("d-" + day)?.scrollIntoView({ block: "start", behavior: motion });
		}
	});
}

const deceased = document.querySelector('.panel input[name="deceased"]');
const gregorian = document.querySelector('.panel input[name="gregorian"]');
if (deceased && gregorian) {
	gregorian.addEventListener("change", () => {
		if (gregorian.checked) {
			deceased.checked = true;
		}
	});
	deceased.addEventListener("change", () => {
		if (!deceased.checked) {
			gregorian.checked = false;
		}
	});
}

for (const input of document.querySelectorAll("[data-autosubmit]")) {
	input.addEventListener("change", () => input.form.submit());
}

const amount = document.getElementById("amount");
const chips = document.querySelectorAll(".chip[data-amount]");
if (amount && chips.length > 0) {
	const sync = () => {
		const value = amount.value.trim();
		for (const chip of chips) {
			chip.setAttribute("aria-pressed", String(chip.dataset.amount === value));
		}
	};

	for (const chip of chips) {
		chip.addEventListener("click", () => {
			amount.value = chip.getAttribute("aria-pressed") === "true" ? "" : chip.dataset.amount;
			sync();
		});
	}
	amount.addEventListener("input", sync);
}

const seriesForm = document.querySelector(".panel-form");
if (seriesForm) {
	const content = seriesForm.querySelector("#content");
	const time = seriesForm.querySelector("#time");
	const outsideBox = seriesForm.querySelector('input[name="outside"]');
	const deceasedBox = seriesForm.querySelector('input[name="deceased"]');
	const gregorianBox = seriesForm.querySelector('input[name="gregorian"]');
	const boxes = [outsideBox, deceasedBox, gregorianBox];
	const isNew = seriesForm.hasAttribute("data-new");
	const locked = seriesForm.querySelector('input[type="hidden"][name="series"]') !== null;
	const radios = [...seriesForm.querySelectorAll('.series-option input[name="series"]')];
	const none = document.getElementById("series-none");
	const step = document.getElementById("series-step");
	const contentHint = document.getElementById("content-series-hint");
	const newHint = document.getElementById("series-new-hint");
	const paid = document.getElementById("series-paid");
	const amountRow = document.getElementById("amount-row");
	const once = document.getElementById("amount-once");
	const amountHint = document.getElementById("amount-hint");
	const amountLabel = seriesForm.querySelector('label[for="amount"]');
	const sharedHint = document.getElementById("shared-hint");
	let typed = content.readOnly ? "" : content.value;
	let flags = content.readOnly ? [false, false, false] : boxes.map((box) => box.checked);

	const shortDate = (date) => {
		const [, month, day] = date.split("-");
		return Number(day) + "." + Number(month);
	};
	const clock = (value) => value.replace(/^0/, "");
	const chosenDay = () => seriesForm.querySelector('input[name="day"]:checked')?.value ?? "";
	const selected = () => radios.find((radio) => radio.checked && radio.value !== "");
	const stepOf = (radio) => radio.nextElementSibling.querySelector(".series-step-big").textContent;

	const optionState = (radio, day) => {
		const days = radio.dataset.days ? radio.dataset.days.split(",") : [];
		const next = radio.dataset.next;
		const step = days.filter((date) => date < day).length + 1;
		if (!day) {
			return next
				? { step: days.length + 1, meta: "następna: " + shortDate(next) + " · " + clock(radio.dataset.time) }
				: { step: days.length, meta: "ma już 30 Mszy", disabled: true };
		}
		if (days.includes(day)) {
			return { step, meta: "już dodana na ten dzień", disabled: true };
		}
		if (day < radio.dataset.first) {
			return { step: 0, meta: "", disabled: true, hidden: true };
		}
		if (!next) {
			return { step: days.length, meta: "ma już 30 Mszy", disabled: true };
		}
		if (day === next) {
			return { step, meta: "na ten dzień · " + clock(radio.dataset.time), kind: "due" };
		}
		if (day > next) {
			return { step, meta: "zaległa od " + shortDate(next), kind: "late" };
		}
		return { step, meta: "uzupełnia przerwę w serii", kind: "gap" };
	};

	const refreshOptions = () => {
		const day = chosenDay();
		for (const radio of radios) {
			if (radio.value === "") {
				continue;
			}
			const state = optionState(radio, day);
			const label = radio.nextElementSibling;
			radio.closest(".series-option").hidden = Boolean(state.hidden);
			radio.disabled = Boolean(state.disabled);
			if (radio.disabled && radio.checked) {
				radio.checked = false;
				none.checked = true;
			}
			label.querySelector(".series-step-big").textContent = state.step ? state.step + "/30" : "";
			const meta = label.querySelector(".series-meta");
			meta.textContent = state.meta;
			meta.className = "series-meta" + (state.kind ? " is-" + state.kind : "");
		}
	};

	const showPaid = (series) => {
		if (series.dataset.paid) {
			const strong = document.createElement("strong");
			strong.textContent = series.dataset.paid;
			paid.replaceChildren("Ofiara za całą serię: ", strong, " – wpisana przy 1/30.");
		} else {
			paid.textContent = "Ofiara za tę serię nie została wpisana – wpisz ją przy 1/30.";
		}
	};

	const applySeries = (moveTime) => {
		if (locked) {
			return;
		}
		const series = selected();
		if (series) {
			if (!content.readOnly) {
				typed = content.value;
				flags = boxes.map((box) => box.checked);
			}
			content.value = series.dataset.content;
			content.readOnly = true;
			outsideBox.checked = series.dataset.outside === "1";
			deceasedBox.checked = true;
			gregorianBox.checked = true;
			showPaid(series);
			if (moveTime && series.dataset.time && time.querySelector('option[value="' + series.dataset.time + '"]')) {
				time.value = series.dataset.time;
				time.dispatchEvent(new Event("change", { bubbles: true }));
			}
		} else if (content.readOnly) {
			content.readOnly = false;
			content.value = typed;
			boxes.forEach((box, i) => {
				box.checked = flags[i];
			});
		}

		const starts = isNew && !series && gregorianBox.checked;
		for (const box of boxes) {
			box.disabled = Boolean(series);
		}
		contentHint.hidden = !series;
		paid.hidden = !series;
		amountRow.hidden = Boolean(series);
		amountHint.hidden = Boolean(series);
		once.hidden = !starts;
		if (newHint) {
			newHint.hidden = !starts;
		}
		amountLabel.textContent = starts ? amountLabel.dataset.seriesLabel : "Wpłata";
		step.hidden = !series && !starts;
		step.textContent = series ? "gregoriańska " + stepOf(series) : "gregoriańska 1/30";
	};

	const refreshShared = () => {
		const mass = document.querySelector("#d-" + chosenDay() + ' .mass[data-time="' + time.value + '"]');
		let others = 0;
		let gregorians = 0;
		if (mass) {
			others = Number(mass.dataset.count);
			gregorians = Number(mass.dataset.gregorian);
			const editing = mass.querySelector(".intentions:not(.outside) .intention.is-editing");
			if (editing) {
				others--;
				if (editing.classList.contains("is-gregorian")) {
					gregorians--;
				}
			}
		}
		const gregorianForm = locked || Boolean(selected()) || gregorianBox.checked;
		sharedHint.hidden = outsideBox.checked || !(others > 0 && (gregorianForm || gregorians > 0));
	};

	seriesForm.addEventListener("change", (event) => {
		const name = event.target.name;
		if (name === "day" && radios.length > 0) {
			refreshOptions();
			applySeries(false);
		}
		if (name === "series") {
			applySeries(true);
		}
		if (name === "gregorian") {
			applySeries(false);
		}
		refreshShared();
	});
}
