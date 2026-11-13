/* The client half of the avatar components.
 *
 * It registers two behaviours with arandu.ui.define, so each runs on the
 * element the component draws and nowhere else: mounted on the first load and on
 * every swap that brings one in, destroyed when a swap takes it away. Nothing
 * here reads an attribute and evaluates it, and nothing is inline, so the page
 * keeps script-src 'self'.
 *
 *   avatar-image   marks the avatar when its photo fails to load, so the drawing
 *                  under it is what shows.
 *   avatar-editor  the cropper. A person picks or drops a photo, drags it to
 *                  frame the square, zooms, and sees the result at three sizes
 *                  before saving. The photo is read with createImageBitmap and
 *                  drawn on a canvas, never put in an <img src> -- the policy
 *                  has no blob: in img-src and does not need one. What is sent
 *                  is the file itself and three fractions, crop_x, crop_y and
 *                  crop_size; the server cuts the picture, and clamps whatever
 *                  arrives.
 *
 * The form works without any of it: a browser without script picks a file and
 * posts it, and the server cuts the largest centred square.
 */
(function () {
	'use strict';

	var arandu = window.arandu;
	if (!arandu || !arandu.ui || typeof arandu.ui.define !== 'function') {
		if (window.console && console.error) {
			console.error('arandu-avatar: arandu.ui is not on the page; the layout has to load ui.js before this script');
		}
		return;
	}
	if (arandu.avatar) return;
	arandu.avatar = { behaviors: ['avatar-image', 'avatar-editor'] };

	/* ---- avatar-image ---------------------------------------------------- */

	arandu.ui.define('avatar-image', {
		mounted: function (ctx) {
			var img = ctx.element.querySelector('img');
			if (!img) return;
			ctx.img = img;
			ctx.fail = function () { ctx.element.setAttribute('data-failed', 'true'); };
			img.addEventListener('error', ctx.fail);
			/* The error may have fired before this script ran. */
			if (img.complete && img.naturalWidth === 0 && img.getAttribute('src')) ctx.fail();
		},
		destroyed: function (ctx) {
			if (ctx.img && ctx.fail) ctx.img.removeEventListener('error', ctx.fail);
		},
	});

	/* ---- avatar-editor --------------------------------------------------- */

	var STAGE = 256; /* the canvas is drawn at this many pixels and scaled by CSS */
	var MIN_ZOOM = 1;
	var MAX_ZOOM = 4;

	function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }

	function fill(template, values) {
		return String(template || '').replace(/:(\w+)/g, function (all, name) {
			return Object.prototype.hasOwnProperty.call(values, name) ? values[name] : all;
		});
	}

	arandu.ui.define('avatar-editor', {
		mounted: function (ctx) {
			var root = ctx.element;
			var props = ctx.props || {};
			var messages = props.messages || {};
			var types = props.types || ['image/jpeg', 'image/png', 'image/webp'];
			var maxBytes = props.maxBytes || 5 * 1024 * 1024;

			var form = root.querySelector('[data-part="upload"]');
			if (!form) return;
			var input = form.querySelector('[data-part="file"]');
			var zone = form.querySelector('[data-part="dropzone"]');
			var cropper = form.querySelector('[data-part="cropper"]');
			var stage = form.querySelector('[data-part="stage"]');
			var canvas = form.querySelector('[data-ref="stage-canvas"]');
			var zoom = form.querySelector('[data-part="zoom"]');
			var problem = form.querySelector('[data-ref="client-error"]');
			var cancel = form.querySelector('[data-ref="cancel"]');
			var save = form.querySelector('[data-ref="submit-photo"]');
			var fields = {
				x: form.querySelector('[data-ref="crop-x"]'),
				y: form.querySelector('[data-ref="crop-y"]'),
				size: form.querySelector('[data-ref="crop-size"]'),
			};
			if (!input || !cropper || !stage || !canvas || !zoom) return;
			var previews = Array.prototype.slice.call(form.querySelectorAll('[data-ref="preview"]'));
			var context2d = canvas.getContext('2d');
			if (!context2d) return;
			canvas.width = STAGE;
			canvas.height = STAGE;

			/* The picture and the frame over it. cx and cy are the centre of the
			 * square, in pixels of the picture; z is the zoom, and the square's
			 * side is the shorter side of the picture divided by it. */
			var state = { bitmap: null, w: 0, h: 0, cx: 0, cy: 0, z: 1, drag: null };

			function say(text) {
				if (!problem) return;
				problem.textContent = text || '';
				if (text) problem.removeAttribute('hidden'); else problem.setAttribute('hidden', '');
			}

			function side() { return Math.min(state.w, state.h) / state.z; }

			function settle() {
				var half = side() / 2;
				state.cx = clamp(state.cx, half, state.w - half);
				state.cy = clamp(state.cy, half, state.h - half);
			}

			function draw() {
				if (!state.bitmap) return;
				settle();
				var s = side();
				var sx = state.cx - s / 2;
				var sy = state.cy - s / 2;
				context2d.clearRect(0, 0, STAGE, STAGE);
				context2d.drawImage(state.bitmap, sx, sy, s, s, 0, 0, STAGE, STAGE);
				previews.forEach(function (p) {
					var c = p.getContext('2d');
					if (!c) return;
					c.clearRect(0, 0, p.width, p.height);
					c.drawImage(state.bitmap, sx, sy, s, s, 0, 0, p.width, p.height);
				});
				if (fields.x) fields.x.value = (sx / state.w).toFixed(5);
				if (fields.y) fields.y.value = (sy / state.h).toFixed(5);
				if (fields.size) fields.size.value = (s / Math.min(state.w, state.h)).toFixed(5);
			}

			/* Nothing to save until a photo is chosen, and nothing to cancel. */
			function ready(on) {
				if (save) save.disabled = !on;
				if (cancel) { if (on) cancel.removeAttribute('hidden'); else cancel.setAttribute('hidden', ''); }
			}
			ready(false);

			function reset() {
				ready(false);
				if (state.bitmap && state.bitmap.close) state.bitmap.close();
				state.bitmap = null;
				input.value = '';
				cropper.setAttribute('hidden', '');
				if (fields.x) fields.x.value = '';
				if (fields.y) fields.y.value = '';
				if (fields.size) fields.size.value = '';
			}

			function accept(file) {
				if (!file) { reset(); return; }
				say('');
				var kind = file.type;
				if (types.indexOf(kind) < 0) {
					reset();
					say(messages.type || '');
					return;
				}
				if (file.size > maxBytes) {
					reset();
					say(fill(messages.size, { mb: String(Math.round(maxBytes / (1024 * 1024) * 10) / 10) }));
					return;
				}
				if (typeof createImageBitmap !== 'function') {
					/* No cropper here: the form still posts the file and the
					 * server cuts the centred square. */
					ready(true);
					return;
				}
				createImageBitmap(file).then(function (bitmap) {
					if (ctx.gone) { if (bitmap.close) bitmap.close(); return; }
					if (state.bitmap && state.bitmap.close) state.bitmap.close();
					state.bitmap = bitmap;
					state.w = bitmap.width;
					state.h = bitmap.height;
					state.cx = state.w / 2;
					state.cy = state.h / 2;
					state.z = MIN_ZOOM;
					zoom.min = String(MIN_ZOOM);
					zoom.max = String(MAX_ZOOM);
					zoom.value = String(MIN_ZOOM);
					cropper.removeAttribute('hidden');
					ready(true);
					draw();
					stage.focus({ preventScroll: true });
				}, function () {
					reset();
					say(messages.read || '');
				});
			}

			ctx.onChange = function () { accept(input.files && input.files[0]); };
			input.addEventListener('change', ctx.onChange);

			/* Dropping a file. dragover has to be cancelled or the browser opens
			 * the file instead. */
			function stop(event) { event.preventDefault(); event.stopPropagation(); }
			ctx.over = function (event) { stop(event); if (zone) zone.setAttribute('data-dragging', 'true'); };
			ctx.leave = function (event) { stop(event); if (zone) zone.removeAttribute('data-dragging'); };
			ctx.drop = function (event) {
				stop(event);
				if (zone) zone.removeAttribute('data-dragging');
				var files = event.dataTransfer && event.dataTransfer.files;
				if (!files || !files.length) return;
				var carried = new DataTransfer();
				carried.items.add(files[0]);
				input.files = carried.files;
				input.dispatchEvent(new Event('change', { bubbles: true }));
			};
			if (zone) {
				zone.addEventListener('dragenter', ctx.over);
				zone.addEventListener('dragover', ctx.over);
				zone.addEventListener('dragleave', ctx.leave);
				zone.addEventListener('drop', ctx.drop);
			}

			/* Moving the picture under the frame: a drag with a pointer, or the
			 * arrow keys. A pixel of drag moves the square by the share of its
			 * side the stage shows per pixel. */
			ctx.down = function (event) {
				if (!state.bitmap) return;
				state.drag = { x: event.clientX, y: event.clientY, cx: state.cx, cy: state.cy };
				if (stage.setPointerCapture) stage.setPointerCapture(event.pointerId);
				event.preventDefault();
			};
			ctx.move = function (event) {
				if (!state.drag) return;
				var shown = stage.getBoundingClientRect().width || STAGE;
				var per = side() / shown;
				state.cx = state.drag.cx - (event.clientX - state.drag.x) * per;
				state.cy = state.drag.cy - (event.clientY - state.drag.y) * per;
				draw();
			};
			ctx.up = function (event) {
				state.drag = null;
				if (stage.releasePointerCapture && event.pointerId !== undefined) {
					try { stage.releasePointerCapture(event.pointerId); } catch (e) { /* not captured */ }
				}
			};
			stage.addEventListener('pointerdown', ctx.down);
			stage.addEventListener('pointermove', ctx.move);
			stage.addEventListener('pointerup', ctx.up);
			stage.addEventListener('pointercancel', ctx.up);

			function setZoom(value) {
				state.z = clamp(value, MIN_ZOOM, MAX_ZOOM);
				zoom.value = String(state.z);
				draw();
			}
			ctx.zoomed = function () { setZoom(parseFloat(zoom.value) || MIN_ZOOM); };
			zoom.addEventListener('input', ctx.zoomed);

			ctx.key = function (event) {
				if (!state.bitmap) return;
				var step = side() * 0.04;
				switch (event.key) {
					case 'ArrowLeft': state.cx -= step; break;
					case 'ArrowRight': state.cx += step; break;
					case 'ArrowUp': state.cy -= step; break;
					case 'ArrowDown': state.cy += step; break;
					case '+': case '=': setZoom(state.z + 0.1); event.preventDefault(); return;
					case '-': case '_': setZoom(state.z - 0.1); event.preventDefault(); return;
					default: return;
				}
				event.preventDefault();
				draw();
			};
			stage.addEventListener('keydown', ctx.key);

			ctx.wheel = function (event) {
				if (!state.bitmap) return;
				event.preventDefault();
				setZoom(state.z * (event.deltaY < 0 ? 1.05 : 0.95));
			};
			stage.addEventListener('wheel', ctx.wheel, { passive: false });

			if (cancel) {
				ctx.cancel = function () { say(''); reset(); };
				cancel.addEventListener('click', ctx.cancel);
			}

			ctx.state = state;
			ctx.parts = { input: input, zone: zone, stage: stage, zoom: zoom, cancel: cancel };
		},

		destroyed: function (ctx) {
			ctx.gone = true;
			var p = ctx.parts;
			if (!p) return;
			p.input.removeEventListener('change', ctx.onChange);
			if (p.zone) {
				p.zone.removeEventListener('dragenter', ctx.over);
				p.zone.removeEventListener('dragover', ctx.over);
				p.zone.removeEventListener('dragleave', ctx.leave);
				p.zone.removeEventListener('drop', ctx.drop);
			}
			p.stage.removeEventListener('pointerdown', ctx.down);
			p.stage.removeEventListener('pointermove', ctx.move);
			p.stage.removeEventListener('pointerup', ctx.up);
			p.stage.removeEventListener('pointercancel', ctx.up);
			p.stage.removeEventListener('keydown', ctx.key);
			p.stage.removeEventListener('wheel', ctx.wheel);
			p.zoom.removeEventListener('input', ctx.zoomed);
			if (p.cancel && ctx.cancel) p.cancel.removeEventListener('click', ctx.cancel);
			if (ctx.state && ctx.state.bitmap && ctx.state.bitmap.close) ctx.state.bitmap.close();
		},
	});
})();
