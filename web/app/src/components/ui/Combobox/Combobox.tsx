import selectionStyles from "../Select/Select.module.css";
import styles from "./Combobox.module.css";
import { Check, ChevronDown, Search } from "lucide-react";
import { Fragment, useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "@/components/ui/Popover";
import type { PopoverContentProps } from "@/components/ui/Popover";
import { TextInput } from "@/components/ui/FormControls";
import { FieldContext, useFieldControlProps } from "@/components/ui/FormControls/fieldContext";
import { classNames } from "@/shared/lib/classNames";
import { nextFocusTarget } from "./focus";

export type ComboboxOption = {
  value: string;
  label: ReactNode;
  textValue?: string;
  description?: ReactNode;
  disabled?: boolean;
  separatorAfter?: boolean;
};

export type ComboboxProps = {
  options: readonly ComboboxOption[];
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  disabled?: boolean;
  required?: boolean;
  name?: string;
  placeholder?: ReactNode;
  selectedLabel?: ReactNode;
  searchPlaceholder: string;
  emptyLabel?: ReactNode;
  loading?: boolean;
  loadingLabel?: ReactNode;
  size?: "sm" | "md";
  triggerClassName?: string;
  triggerProps?: Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children" | "className">;
  contentClassName?: string;
  contentProps?: Omit<PopoverContentProps, "children" | "className">;
};

function searchText(option: ComboboxOption): string {
  return (
    option.textValue ??
    (typeof option.label === "string" || typeof option.label === "number" ? String(option.label) : option.value)
  );
}

export function Combobox({
  options,
  value,
  defaultValue = "",
  onValueChange,
  open,
  defaultOpen = false,
  onOpenChange,
  disabled = false,
  required = false,
  name,
  placeholder,
  selectedLabel,
  searchPlaceholder,
  emptyLabel,
  loading = false,
  loadingLabel,
  size = "md",
  triggerClassName,
  triggerProps = {},
  contentClassName,
  contentProps,
}: ComboboxProps) {
  const [internalValue, setInternalValue] = useState(defaultValue);
  const [internalOpen, setInternalOpen] = useState(defaultOpen);
  const [query, setQuery] = useState("");
  const [lockedSide, setLockedSide] = useState<"top" | "bottom" | null>(null);
  const [highlightedValue, setHighlightedValue] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [inDialog, setInDialog] = useState(false);
  const attachTrigger = useCallback((node: HTMLButtonElement | null) => {
    triggerRef.current = node;
    if (node) setInDialog(Boolean(node.closest('[role="dialog"]')));
  }, []);
  const inputRef = useRef<HTMLInputElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const composing = useRef(false);
  const restoreFocus = useRef(true);
  const tabTarget = useRef<HTMLElement | null>(null);
  const listId = useId();
  const selectedValue = value ?? internalValue;
  const isOpen = !disabled && (open ?? internalOpen);
  const selected = options.find((option) => option.value === selectedValue);
  const fieldProps = useFieldControlProps(triggerProps);
  const filtered = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    return normalized
      ? options.filter((option) => option.value !== "" && searchText(option).toLocaleLowerCase().includes(normalized))
      : options;
  }, [options, query]);
  const enabled = loading ? [] : filtered.filter((option) => !option.disabled);
  const activeValue = enabled.some((option) => option.value === highlightedValue)
    ? highlightedValue
    : (enabled[0]?.value ?? null);
  const activeIndex = filtered.findIndex((option) => option.value === activeValue);
  const activeId = activeIndex >= 0 ? `${listId}-${activeIndex}` : undefined;

  function changeOpen(next: boolean) {
    if (disabled && next) return;
    setInternalOpen(next);
    setLockedSide(null);
    if (next) {
      setQuery("");
      setHighlightedValue(selected?.disabled ? null : selectedValue);
      restoreFocus.current = true;
      tabTarget.current = null;
    }
    onOpenChange?.(next);
  }

  function commit(option: ComboboxOption) {
    if (disabled || loading || option.disabled) return;
    setInternalValue(option.value);
    onValueChange?.(option.value);
    changeOpen(false);
  }

  function moveHighlight(offset: number) {
    if (!enabled.length) return;
    const index = enabled.findIndex((option) => option.value === activeValue);
    setHighlightedValue(enabled[(index + offset + enabled.length) % enabled.length].value);
  }

  useEffect(() => {
    if (isOpen && activeId) document.getElementById(activeId)?.scrollIntoView({ block: "nearest" });
  }, [activeId, isOpen]);

  useEffect(() => {
    if (!isOpen || lockedSide) return;
    const frame = requestAnimationFrame(() => {
      const side = contentRef.current?.dataset.side;
      if (side === "top" || side === "bottom") setLockedSide(side);
    });
    return () => cancelAnimationFrame(frame);
  }, [isOpen, lockedSide]);

  return (
    <PopoverRoot open={isOpen} onOpenChange={changeOpen} modal={inDialog}>
      <PopoverTrigger asChild>
        <button
          data-csg-control="combobox"
          {...fieldProps}
          ref={attachTrigger}
          type="button"
          className={classNames(
            classNames(
              classNames(selectionStyles["csg-select-trigger"], "csg-select-trigger"),
              "inline-flex",
              "w-full",
              "items-center",
              "justify-between",
              "gap-2",
            ),
            selectionStyles[`csg-select-trigger-${size}`],
            triggerClassName,
          )}
          disabled={disabled}
          role="combobox"
          aria-haspopup="listbox"
          aria-controls={isOpen ? listId : undefined}
          aria-expanded={isOpen}
          aria-hidden={isOpen || undefined}
          aria-required={required || fieldProps["aria-required"]}
          data-placeholder={!selected && !selectedValue ? "" : undefined}
          onKeyDown={(event) => {
            triggerProps.onKeyDown?.(event);
            if (event.defaultPrevented) return;
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              event.preventDefault();
              changeOpen(true);
              if (event.key === "ArrowUp") setHighlightedValue(enabled.at(-1)?.value ?? null);
            }
          }}
        >
          <span
            className={classNames(
              classNames(selectionStyles["csg-select-value"], "csg-select-value"),
              "min-w-0",
              "flex-1",
              "truncate",
            )}
          >
            {selectedLabel ?? selected?.label ?? (selectedValue || placeholder)}
          </span>
          <ChevronDown
            className={classNames(classNames(selectionStyles["csg-select-chevron"], "csg-select-chevron"), "shrink-0")}
            aria-hidden="true"
            size={16}
          />
        </button>
      </PopoverTrigger>
      {name || required ? (
        <select
          data-csg-control="native-select"
          className={styles["csg-combobox-native"]}
          tabIndex={-1}
          aria-hidden="true"
          name={name}
          value={selectedValue}
          required={required}
          disabled={disabled}
          onChange={(event) => {
            const option = options.find((item) => item.value === event.target.value);
            if (option) commit(option);
          }}
          onInvalid={(event) => {
            event.preventDefault();
            triggerRef.current?.focus();
          }}
        >
          {!options.some((option) => option.value === "") ? <option value="" /> : null}
          {selectedValue && !selected ? <option value={selectedValue}>{selectedValue}</option> : null}
          {options.map((option) => (
            <option key={option.value} value={option.value} disabled={option.disabled}>
              {searchText(option)}
            </option>
          ))}
        </select>
      ) : null}
      <PopoverContent
        {...contentProps}
        side={lockedSide ?? contentProps?.side ?? "bottom"}
        collisionPadding={contentProps?.collisionPadding ?? 12}
        ref={contentRef}
        role="presentation"
        portalContainer={contentProps?.portalContainer}
        className={classNames(styles["csg-combobox-content"], contentClassName)}
        onOpenAutoFocus={(event) => {
          contentProps?.onOpenAutoFocus?.(event);
          if (event.defaultPrevented) return;
          event.preventDefault();
          inputRef.current?.focus();
        }}
        onCloseAutoFocus={(event) => {
          contentProps?.onCloseAutoFocus?.(event);
          if (event.defaultPrevented) return;
          event.preventDefault();
          if (tabTarget.current?.isConnected) tabTarget.current.focus();
          else if (restoreFocus.current) triggerRef.current?.focus();
          tabTarget.current = null;
        }}
        onEscapeKeyDown={(event) => {
          contentProps?.onEscapeKeyDown?.(event);
          if (!event.defaultPrevented) {
            event.preventDefault();
            changeOpen(false);
          }
        }}
        onInteractOutside={(event) => {
          contentProps?.onInteractOutside?.(event);
          if (!event.defaultPrevented) restoreFocus.current = false;
        }}
      >
        <div className={styles["csg-combobox-search"]} data-invalid={fieldProps["aria-invalid"] || undefined}>
          <Search size={15} aria-hidden="true" />
          <FieldContext.Provider value={null}>
            <TextInput
              ref={inputRef}
              id={`${listId}-search`}
              variant="plain"
              role="combobox"
              aria-label={searchPlaceholder}
              aria-labelledby={undefined}
              aria-expanded="true"
              aria-autocomplete="list"
              aria-controls={listId}
              aria-describedby={fieldProps["aria-describedby"]}
              aria-invalid={fieldProps["aria-invalid"]}
              aria-required={required || fieldProps["aria-required"]}
              aria-activedescendant={activeId}
              autoComplete="off"
              placeholder={searchPlaceholder}
              value={query}
              onChange={(event) => {
                setQuery(event.currentTarget.value);
                setHighlightedValue(null);
              }}
              onCompositionStart={() => {
                composing.current = true;
              }}
              onCompositionEnd={() => {
                composing.current = false;
              }}
              onKeyDown={(event) => {
                if (composing.current || event.nativeEvent.isComposing || event.keyCode === 229) return;
                if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                  event.preventDefault();
                  event.stopPropagation();
                  moveHighlight(event.key === "ArrowDown" ? 1 : -1);
                } else if (event.key === "Enter") {
                  event.preventDefault();
                  event.stopPropagation();
                  const option = enabled.find((item) => item.value === activeValue);
                  if (option) commit(option);
                } else if (event.key === "Tab") {
                  restoreFocus.current = false;
                  tabTarget.current = triggerRef.current
                    ? nextFocusTarget(triggerRef.current, contentRef.current, event.shiftKey)
                    : null;
                  if (tabTarget.current) event.preventDefault();
                  changeOpen(false);
                }
              }}
            />
          </FieldContext.Provider>
        </div>
        <div
          className={styles["csg-combobox-options"]}
          role="listbox"
          id={listId}
          aria-label={triggerProps["aria-label"] ?? searchPlaceholder}
          aria-busy={loading}
        >
          {loading ? (
            <div className={selectionStyles["csg-select-empty"]} role="status">
              {loadingLabel}
            </div>
          ) : filtered.length ? (
            filtered.map((option, index) => (
              <Fragment key={option.value}>
                <div
                  id={`${listId}-${index}`}
                  role="option"
                  aria-selected={option.value === selectedValue}
                  aria-disabled={option.disabled || undefined}
                  data-highlighted={option.value === activeValue ? "" : undefined}
                  data-disabled={option.disabled ? "" : undefined}
                  className={classNames(
                    selectionStyles["csg-select-item"],
                    "relative flex w-full items-center rounded-md py-2 pr-3 pl-8",
                  )}
                  onPointerMove={() => {
                    if (!option.disabled) setHighlightedValue(option.value);
                  }}
                  onPointerDown={(event) => event.preventDefault()}
                  onClick={() => commit(option)}
                >
                  {option.value === selectedValue ? (
                    <Check
                      className={classNames(selectionStyles["csg-select-item-indicator"], "absolute left-2")}
                      size={16}
                      aria-hidden="true"
                    />
                  ) : null}
                  <span className={selectionStyles["csg-select-item-copy"]}>
                    <span className={styles["csg-combobox-label"]}>{option.label}</span>
                    {option.description ? (
                      <span className={selectionStyles["csg-select-item-description"]}>{option.description}</span>
                    ) : null}
                  </span>
                </div>
                {option.separatorAfter && index < filtered.length - 1 ? (
                  <div role="separator" className={classNames(selectionStyles["csg-select-separator"], "my-1")} />
                ) : null}
              </Fragment>
            ))
          ) : (
            <div className={selectionStyles["csg-select-empty"]} role="status">
              {emptyLabel}
            </div>
          )}
        </div>
      </PopoverContent>
    </PopoverRoot>
  );
}
