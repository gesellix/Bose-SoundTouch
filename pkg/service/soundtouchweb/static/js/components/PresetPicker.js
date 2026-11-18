import { h } from 'preact';
import { useState, useEffect, useLayoutEffect, useRef } from 'preact/hooks';
import htm from 'htm';

const html = htm.bind(h);

const SLOTS = [1, 2, 3, 4, 5, 6];

// PresetPicker is the ★ button plus its 1–6 slot popover, shared by the
// now-playing card and the Library rows (issue 700). It owns the popover
// state and the save feedback; what a save *does* is the caller's business.
//
// onSave(slot) may return a thenable for inline feedback. Its result must
// reflect confirmation of the slot from the speaker.
export function PresetPicker({
    onSave,
    mappedSlot = null,
    label = 'Save as preset',
    title,
    wrapClass = 'preset-picker-wrap',
    buttonClass = 'preset-picker-btn',
    overlayClass = 'preset-picker-overlay',
}) {
    const [open, setOpen] = useState(false);
    const [savingSlot, setSavingSlot] = useState(null);
    const [savedSlot, setSavedSlot] = useState(null);
    const [error, setError] = useState(null);
    const wrapRef = useRef(null);
    const mounted = useRef(true);
    const saving = useRef(false);
    const closeTimer = useRef(null);

    useLayoutEffect(() => () => {
        mounted.current = false;
        clearTimeout(closeTimer.current);
    }, []);

    // Close the popover on an outside click.
    useEffect(() => {
        if (!open) return;

        function onDocClick(e) {
            if (!wrapRef.current?.contains(e.target)) setOpen(false);
        }

        document.addEventListener('click', onDocClick, true);

        return () => document.removeEventListener('click', onDocClick, true);
    }, [open]);

    function save(slotId) {
        if (saving.current) return;
        const pending = onSave?.(slotId);

        if (!pending?.then) {
            // The caller reports the outcome itself.
            setOpen(false);

            return;
        }

        saving.current = true;
        clearTimeout(closeTimer.current);
        setSavedSlot(null);
        setError(null);
        setSavingSlot(slotId);
        pending.then(() => {
            if (!mounted.current) return;
            saving.current = false;
            setSavingSlot(null);
            setSavedSlot(slotId);
            closeTimer.current = setTimeout(() => {
                setSavedSlot(null);
                setOpen(false);
            }, 900);
        }, reason => {
            if (!mounted.current) return;
            saving.current = false;
            setSavingSlot(null);
            setError({ slot: slotId, message: reason?.message || 'Could not verify the preset change.' });
        });
    }

    const isMapped = mappedSlot !== null && mappedSlot !== undefined;
    const buttonTitle = title || (isMapped
        ? `Saved as preset ${mappedSlot}. Save again to update.`
        : label);

    return html`
        <div class=${wrapClass} ref=${wrapRef}>
            <button
                class="${buttonClass} ${isMapped ? 'mapped' : ''} ${open ? 'open' : ''}"
                onClick=${e => { e.stopPropagation(); setOpen(o => !o); }}
                title=${buttonTitle}
                aria-label=${buttonTitle}
                aria-expanded=${open ? 'true' : 'false'}
            >★</button>
            ${open && html`
                <div class=${overlayClass}>
                    <div class="preset-picker-label">${label}</div>
                    <div class="preset-picker-slots">
                        ${SLOTS.map(n => html`
                            <button
                                key=${n}
                                class="preset-picker-slot ${savingSlot === n ? 'saving' : savedSlot === n ? 'saved' : error?.slot === n ? 'error' : ''}"
                                onClick=${e => { e.stopPropagation(); save(n); }}
                                disabled=${savingSlot !== null}
                            >${savedSlot === n ? '✓' : error?.slot === n ? '✗' : n}</button>
                        `)}
                    </div>
                    ${error && html`<div role="alert" style="color:var(--danger);font-size:.8rem;max-width:15rem">${error.message}</div>`}
                </div>
            `}
        </div>
    `;
}
