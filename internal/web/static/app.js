document.addEventListener("keydown", (event) => {
	if (event.key !== "Escape") {
		return;
	}

	const close = document.querySelector(".panel-close");
	if (close) {
		window.location.href = close.href;
	}
});

const selectedDay = document.querySelector(".day.is-selected");
if (selectedDay) {
	selectedDay.scrollIntoView({ block: "start" });
}

const field = document.querySelector('.panel [aria-invalid="true"]') ?? document.getElementById("content");
if (field && document.activeElement !== field) {
	field.focus({ preventScroll: true });
}
