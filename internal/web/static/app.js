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

const field = document.querySelector('.panel [aria-invalid="true"]') ?? document.getElementById("content") ?? document.getElementById("feast");
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
