const draftForm = document.querySelector(".panel-form");
const draftToken = draftForm ? draftForm.dataset.draftToken || newDraftToken() : "";
let stopDraft = () => {};

function newDraftToken() {
	if (window.crypto && typeof window.crypto.randomUUID === "function") {
		return window.crypto.randomUUID();
	}
	return Date.now().toString(36) + Math.random().toString(36).slice(2);
}

const leave = (link) => {
	if (!draftForm || !link.hasAttribute("data-discard")) {
		window.location.href = link.href;
		return;
	}

	stopDraft();
	fetch("/draft/discard", { method: "POST", body: new URLSearchParams({ draft_token: draftToken }) })
		.catch(() => {})
		.finally(() => {
			window.location.href = link.href;
		});
};

document.addEventListener("keydown", (event) => {
	if (event.key !== "Escape") {
		return;
	}

	const close = document.querySelector("[data-close]");
	if (close) {
		leave(close);
	}
});

if (draftForm) {
	const tokenInput = document.createElement("input");
	tokenInput.type = "hidden";
	tokenInput.name = "draft_token";
	tokenInput.value = draftToken;
	draftForm.append(tokenInput);

	let timer = 0;
	let dirty = false;
	stopDraft = () => {
		clearTimeout(timer);
		dirty = false;
	};

	const send = (keepalive) => {
		clearTimeout(timer);
		if (!dirty) {
			return;
		}
		dirty = false;

		const data = new URLSearchParams(new FormData(draftForm));
		if (draftForm.dataset.edit) {
			data.set("edit", draftForm.dataset.edit);
		}
		fetch("/draft", { method: "POST", body: data, keepalive: keepalive === true }).catch(() => {});
	};

	const later = () => {
		dirty = true;
		clearTimeout(timer);
		timer = setTimeout(send, 400);
	};

	draftForm.addEventListener("input", later);
	draftForm.addEventListener("change", later);
	draftForm.addEventListener("submit", () => stopDraft());
	window.addEventListener("pagehide", () => send(true));

	for (const link of draftForm.querySelectorAll("[data-discard]")) {
		link.addEventListener("click", (event) => {
			event.preventDefault();
			leave(link);
		});
	}
}

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
	input.addEventListener("change", () => {
		try {
			if (input.id) {
				sessionStorage.setItem("autosubmit-focus", input.id);
			}
		} catch {}
		input.form.submit();
	});
}

const busyForms = document.querySelectorAll("form[data-busy]");
for (const form of busyForms) {
	form.addEventListener("submit", () => {
		const button = form.querySelector('button[type="submit"]');
		if (!button || button.disabled) {
			return;
		}
		button.dataset.label = button.innerHTML;
		button.style.minWidth = button.offsetWidth + "px";
		button.disabled = true;
		button.classList.add("is-busy");
		button.setAttribute("aria-busy", "true");
		const spinner = document.createElement("span");
		spinner.className = "spinner";
		spinner.setAttribute("aria-hidden", "true");
		const icon = button.querySelector(".bar-action-icon");
		const text = button.querySelector(".bar-action-text");
		if (icon && text) {
			text.style.width = text.offsetWidth + "px";
			button.dataset.title = button.title;
			button.title = form.dataset.busy;
			icon.replaceChildren(spinner);
			text.textContent = form.dataset.busy;
		} else {
			button.replaceChildren(spinner, form.dataset.busy);
		}
	});
}
window.addEventListener("pageshow", (event) => {
	if (!event.persisted) {
		return;
	}
	for (const button of document.querySelectorAll("form[data-busy] button.is-busy")) {
		button.innerHTML = button.dataset.label;
		if (button.dataset.title) {
			button.title = button.dataset.title;
		}
		button.style.minWidth = "";
		button.disabled = false;
		button.classList.remove("is-busy");
		button.removeAttribute("aria-busy");
	}
});

const pickerMonths = ["Styczeń", "Luty", "Marzec", "Kwiecień", "Maj", "Czerwiec", "Lipiec", "Sierpień", "Wrzesień", "Październik", "Listopad", "Grudzień"];
const parseDay = (text) => {
	const [year, month, day] = text.split("-").map(Number);
	return new Date(year, month - 1, day);
};
const isoDay = (date) => date.getFullYear() + "-" + String(date.getMonth() + 1).padStart(2, "0") + "-" + String(date.getDate()).padStart(2, "0");
const dayLabel = (date) => date.getDate() + "." + (date.getMonth() + 1) + "." + date.getFullYear();

const datePicker = (input) => {
	const form = input.form;
	const radio = input.dataset.pickerRadio ? document.getElementById(input.dataset.pickerRadio) : null;
	const first = parseDay(input.min);
	const last = parseDay(input.max);
	const today = input.dataset.today ? parseDay(input.dataset.today) : null;
	const rangeStart = input.dataset.rangeStart ? parseDay(input.dataset.rangeStart) : null;
	const rangeEnd = input.dataset.rangeEnd ? parseDay(input.dataset.rangeEnd) : null;
	let chosen = input.value ? parseDay(input.value) : input.hasAttribute("data-picker-stay") ? last : first;
	const previous = form.querySelector('input[name="period"]:checked');
	let shown = new Date(chosen.getFullYear(), chosen.getMonth(), 1);
	let hover = null;

	const element = (tag, className, text) => {
		const node = document.createElement(tag);
		node.className = className;
		if (text !== undefined) {
			node.textContent = text;
		}
		return node;
	};

	const button = element("button", "until-button");
	button.type = "button";
	button.setAttribute("aria-haspopup", "dialog");
	button.setAttribute("aria-expanded", "false");
	button.setAttribute("aria-label", input.getAttribute("aria-label") + ": " + (input.dataset.datePicker || "nie wybrano"));
	button.innerHTML = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true" focusable="false"><rect x="3" y="4" width="18" height="17" rx="2"/><path d="M3 9h18M8 2v4M16 2v4"/></svg><span></span>';
	button.querySelector("span").textContent = input.dataset.datePicker || "wybierz datę";
	button.classList.toggle("is-empty", !input.dataset.datePicker);

	const pop = element("div", "until-pop");
	pop.hidden = true;
	pop.setAttribute("role", "dialog");
	pop.setAttribute("aria-label", input.getAttribute("aria-label"));

	input.hidden = true;
	input.after(button, pop);

	const paint = () => {
		const point = hover || chosen;
		const from = rangeStart || point;
		const to = rangeEnd || point;
		for (const day of pop.querySelectorAll(".until-day[data-date]")) {
			const date = parseDay(day.dataset.date);
			day.classList.toggle("is-range", date >= from && date <= to);
			day.classList.toggle("is-start", date.getTime() === from.getTime());
			day.classList.toggle("is-end", date.getTime() === to.getTime());
			day.classList.toggle("is-chosen", date.getTime() === point.getTime());
		}
	};

	const render = (focus) => {
		pop.replaceChildren();

		const firstMonth = new Date(first.getFullYear(), first.getMonth(), 1);
		const lastMonth = new Date(last.getFullYear(), last.getMonth(), 1);
		const head = element("div", "until-head");
		const navs = [
			["«", "Rok wcześniej", -12],
			["‹", "Poprzedni miesiąc", -1],
			["›", "Następny miesiąc", 1],
			["»", "Rok później", 12],
		].map(([text, name, months], index) => {
			const nav = element("button", "until-nav", text);
			nav.type = "button";
			nav.setAttribute("aria-label", name);
			const target = new Date(shown.getFullYear(), shown.getMonth() + months, 1);
			nav.disabled = months < 0 ? shown <= firstMonth : shown >= lastMonth;
			nav.addEventListener("click", () => {
				shown = target < firstMonth ? firstMonth : target > lastMonth ? lastMonth : target;
				render();
				const again = pop.querySelectorAll(".until-nav")[index];
				(again.disabled ? pop.querySelector(".until-day:not(:disabled)") : again)?.focus({ preventScroll: true });
			});
			return nav;
		});
		head.append(navs[0], navs[1], element("span", "until-title", pickerMonths[shown.getMonth()] + " " + shown.getFullYear()), navs[2], navs[3]);

		const weekdays = element("div", "until-weekdays");
		for (const name of ["Pn", "Wt", "Śr", "Cz", "Pt", "So", "Nd"]) {
			weekdays.append(element("span", "", name));
		}

		const grid = element("div", "until-grid");
		for (let i = 0; i < (shown.getDay() + 6) % 7; i++) {
			grid.append(element("span", ""));
		}
		for (let date = new Date(shown); date.getMonth() === shown.getMonth(); date.setDate(date.getDate() + 1)) {
			const day = element("button", "until-day", String(date.getDate()));
			day.type = "button";
			day.dataset.date = isoDay(date);
			day.disabled = date < first || date > last;
			day.classList.toggle("is-today", today !== null && date.getTime() === today.getTime());
			day.setAttribute("aria-label", dayLabel(date));
			grid.append(day);
		}

		pop.append(head, weekdays, grid);
		if (input.dataset.pickerNote) {
			pop.append(element("p", "until-foot", input.dataset.pickerNote));
		}
		paint();

		if (focus) {
			(pop.querySelector('.until-day[data-date="' + isoDay(focus) + '"]:not(:disabled)') || pop.querySelector(".until-day:not(:disabled)"))?.focus({ preventScroll: true });
		}
	};

	const open = () => {
		for (const other of document.querySelectorAll(".until-pop")) {
			if (other !== pop && !other.hidden) {
				other.dispatchEvent(new Event("close"));
			}
		}
		shown = new Date(chosen.getFullYear(), chosen.getMonth(), 1);
		pop.hidden = false;
		button.setAttribute("aria-expanded", "true");
		render(chosen);
	};

	const close = (focusButton) => {
		if (pop.hidden) {
			return;
		}
		pop.hidden = true;
		hover = null;
		button.setAttribute("aria-expanded", "false");
		if (radio && previous) {
			previous.checked = true;
		}
		if (focusButton) {
			button.focus();
		}
	};

	const pick = (date) => {
		input.value = isoDay(date);
		if (input.hasAttribute("data-picker-stay")) {
			chosen = date;
			input.dataset.datePicker = dayLabel(date);
			button.querySelector("span").textContent = dayLabel(date);
			button.classList.remove("is-empty");
			button.setAttribute("aria-label", input.getAttribute("aria-label") + ": " + dayLabel(date));
			close(true);
			input.dispatchEvent(new Event("change", { bubbles: true }));
			return;
		}
		if (radio) {
			radio.checked = true;
		}
		try {
			sessionStorage.setItem("autosubmit-focus", radio ? radio.id : input.id);
		} catch {}
		form.submit();
	};

	pop.addEventListener("close", () => close(false));
	button.addEventListener("click", () => (pop.hidden ? open() : close(true)));
	radio?.addEventListener("change", open);

	pop.addEventListener("click", (event) => {
		const day = event.target.closest(".until-day");
		if (day && !day.disabled) {
			pick(parseDay(day.dataset.date));
		}
	});
	pop.addEventListener("focusin", (event) => {
		const day = event.target.closest(".until-day");
		if (day && !day.disabled) {
			hover = parseDay(day.dataset.date);
			paint();
		}
	});
	pop.addEventListener("mouseover", (event) => {
		const day = event.target.closest(".until-day");
		hover = day && !day.disabled ? parseDay(day.dataset.date) : null;
		paint();
	});
	pop.addEventListener("mouseleave", () => {
		hover = null;
		paint();
	});
	pop.addEventListener("keydown", (event) => {
		if (event.key === "Escape") {
			event.stopPropagation();
			close(true);
			return;
		}

		const steps = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 };
		const day = event.target.closest(".until-day");
		if (!day || !(event.key in steps)) {
			return;
		}
		event.preventDefault();
		const date = parseDay(day.dataset.date);
		date.setDate(date.getDate() + steps[event.key]);
		if (date < first || date > last) {
			return;
		}
		if (date.getMonth() !== shown.getMonth() || date.getFullYear() !== shown.getFullYear()) {
			shown = new Date(date.getFullYear(), date.getMonth(), 1);
			render(date);
			return;
		}
		pop.querySelector('.until-day[data-date="' + isoDay(date) + '"]').focus({ preventScroll: true });
	});

	document.addEventListener("click", (event) => {
		const path = event.composedPath();
		if (!pop.hidden && !path.includes(pop) && !path.includes(button) && !(radio && path.includes(radio.closest("label")))) {
			close(false);
		}
	});
};

for (const input of document.querySelectorAll("[data-date-picker]")) {
	datePicker(input);
}

try {
	const id = sessionStorage.getItem("autosubmit-focus");
	sessionStorage.removeItem("autosubmit-focus");
	document.getElementById(id ?? "")?.focus();
} catch {}

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
	const acceptedField = document.getElementById("accepted-field");
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
		if (acceptedField) {
			acceptedField.hidden = Boolean(series);
		}
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

const updateProgress = document.querySelector("[data-update-progress]");
if (updateProgress) {
	const bar = updateProgress.querySelector(".update-bar");
	const fill = updateProgress.querySelector(".update-bar-fill");
	const label = updateProgress.querySelector(".update-label");
	const failed = updateProgress.querySelector(".update-failed");
	const note = updateProgress.querySelector(".update-note");
	const error = updateProgress.querySelector("[data-update-error]");

	const show = (status) => {
		fill.style.width = status.percent + "%";
		bar.setAttribute("aria-valuenow", String(status.percent));
		label.textContent = status.label;
		for (const step of status.steps) {
			const item = updateProgress.querySelector('[data-step="' + step.stage + '"]');
			if (item) {
				item.className = "is-" + step.state;
			}
		}
		failed.hidden = !status.failed;
		note.hidden = status.failed;
		error.textContent = status.error;
	};

	const poll = () => {
		fetch(updateProgress.dataset.updateProgress, { cache: "no-store" })
			.then((response) => response.json())
			.then((status) => {
				show(status);
				if (!status.failed && status.stage !== "") {
					setTimeout(poll, 500);
				}
			})
			.catch(() => setTimeout(poll, 1500));
	};

	if (failed.hidden) {
		setTimeout(poll, 500);
	}
}
