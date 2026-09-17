import { EditorContent, useEditor, useEditorState, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import {
  BoldIcon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  StrikethroughIcon,
  UnderlineIcon,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";

import { cn } from "@/lib/utils";

/** Same cap the form schema and the API enforce on the text of a description. */
export const DESCRIPTION_MAX_CHARS = 2000;

/** The counter stays visible (not just on focus) once the text gets this close to the cap. */
const NEAR_LIMIT_CHARS = 1800;

const LINK_PATTERN = /^(?:https?:\/\/|mailto:)\S+$/i;
const LINK_HINT = "Liên kết phải bắt đầu bằng http://, https:// hoặc mailto:";

export interface TaskDescriptionEditorProps {
  id: string;
  value: string;
  onChange: (html: string) => void;
  onBlur?: () => void;
  /** Id of the visible "Mô tả" label; a contenteditable cannot be targeted by `<label for>`. */
  labelId: string;
  invalid: boolean;
  describedBy?: string;
}

/**
 * Text of the document the way the API counts it: no separator between
 * blocks and nothing for a hard break, so the counter shows exactly what the
 * server would measure against its limit.
 */
function characterCount(editor: Editor): number {
  const { doc } = editor.state;
  return Array.from(doc.textBetween(0, doc.content.size, "", "")).length;
}

function editorAttributes(
  props: Pick<TaskDescriptionEditorProps, "id" | "labelId" | "invalid" | "describedBy">,
) {
  const attributes: Record<string, string> = {
    id: props.id,
    role: "textbox",
    "aria-multiline": "true",
    "aria-labelledby": props.labelId,
    "aria-invalid": String(props.invalid),
    class: "prose-task min-h-[96px] px-3 py-2.5 text-[14.5px] text-ink-700 outline-none",
  };
  if (props.describedBy) attributes["aria-describedby"] = props.describedBy;
  return attributes;
}

/**
 * The rich-text editor for a task description. Loaded lazily by the form
 * modal (TipTap is the largest dependency of the tasks feature) and driven
 * as a controlled field: `value` is the HTML the form holds, `onChange`
 * receives `""` for an empty document so the API clears the description.
 */
export function TaskDescriptionEditor({
  id,
  value,
  onChange,
  onBlur,
  labelId,
  invalid,
  describedBy,
}: TaskDescriptionEditorProps) {
  // What this editor last handed to the form, so an echo of our own change
  // does not reset the document (and the caret) through `setContent`.
  const lastEmittedRef = useRef(value);
  // Whether focus is anywhere inside the bordered field — the toolbar, the
  // link panel, or the text itself — so the counter can show while any of
  // them is in use, not just the contenteditable.
  const [focusWithin, setFocusWithin] = useState(false);

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: false,
        blockquote: false,
        codeBlock: false,
        code: false,
        horizontalRule: false,
        dropcursor: false,
        gapcursor: false,
        // http, https and mailto are linkify's built-in schemes; listing
        // them under `protocols` only re-registers them with a warning.
        link: { openOnClick: false, autolink: true, defaultProtocol: "https" },
      }),
    ],
    content: value,
    editorProps: { attributes: editorAttributes({ id, labelId, invalid, describedBy }) },
    onUpdate: ({ editor: current }) => {
      const html = current.isEmpty ? "" : current.getHTML();
      lastEmittedRef.current = html;
      onChange(html);
    },
    onBlur: () => onBlur?.(),
  });

  useEffect(() => {
    if (!editor || value === lastEmittedRef.current) return;
    lastEmittedRef.current = value;
    editor.commands.setContent(value, { emitUpdate: false });
  }, [editor, value]);

  useEffect(() => {
    editor?.setOptions({
      editorProps: { attributes: editorAttributes({ id, labelId, invalid, describedBy }) },
    });
  }, [editor, id, labelId, invalid, describedBy]);

  const state = useEditorState({
    editor,
    selector: ({ editor: current }) =>
      current
        ? {
            bold: current.isActive("bold"),
            italic: current.isActive("italic"),
            underline: current.isActive("underline"),
            strike: current.isActive("strike"),
            bulletList: current.isActive("bulletList"),
            orderedList: current.isActive("orderedList"),
            link: current.isActive("link"),
            count: characterCount(current),
          }
        : null,
  });

  const [linkPanel, setLinkPanel] = useState<{ url: string; error: string | null } | null>(null);

  if (!editor || !state) return null;

  const openLinkPanel = () => {
    const current = editor.getAttributes("link") as { href?: string };
    setLinkPanel({ url: current.href ?? "", error: null });
  };

  // The URL input unmounts with the panel; without this, focus would fall
  // to <body> and a keyboard user would have to tab back from the top.
  const closeLinkPanel = () => {
    setLinkPanel(null);
    editor.commands.focus();
  };

  const applyLink = () => {
    if (!linkPanel) return;
    const href = linkPanel.url.trim();
    if (!LINK_PATTERN.test(href)) {
      setLinkPanel({ url: linkPanel.url, error: LINK_HINT });
      return;
    }
    const chain = editor.chain().focus().extendMarkRange("link");
    if (editor.state.selection.empty && !editor.isActive("link")) {
      // Nothing to wrap: insert the URL itself as the link text.
      chain.insertContent({ type: "text", text: href, marks: [{ type: "link", attrs: { href } }] });
    } else {
      chain.setLink({ href });
    }
    chain.run();
    setLinkPanel(null);
  };

  const removeLink = () => {
    editor.chain().focus().extendMarkRange("link").unsetLink().run();
    setLinkPanel(null);
  };

  const overLimit = state.count > DESCRIPTION_MAX_CHARS;
  const nearLimit = state.count >= NEAR_LIMIT_CHARS;
  const showCounter = focusWithin || nearLimit;

  return (
    <div
      onFocus={() => setFocusWithin(true)}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) {
          setFocusWithin(false);
        }
      }}
      className={cn(
        "rounded-[14px] border border-line-200 bg-white focus-within:border-mint-400",
        invalid && "border-coral-400",
      )}
    >
      <Toolbar
        editor={editor}
        items={[
          {
            label: "Đậm",
            icon: BoldIcon,
            pressed: state.bold,
            onClick: () => editor.chain().focus().toggleBold().run(),
          },
          {
            label: "Nghiêng",
            icon: ItalicIcon,
            pressed: state.italic,
            onClick: () => editor.chain().focus().toggleItalic().run(),
          },
          {
            label: "Gạch chân",
            icon: UnderlineIcon,
            pressed: state.underline,
            onClick: () => editor.chain().focus().toggleUnderline().run(),
          },
          {
            label: "Gạch ngang",
            icon: StrikethroughIcon,
            pressed: state.strike,
            onClick: () => editor.chain().focus().toggleStrike().run(),
          },
          {
            label: "Danh sách chấm",
            icon: ListIcon,
            pressed: state.bulletList,
            onClick: () => editor.chain().focus().toggleBulletList().run(),
          },
          {
            label: "Danh sách số",
            icon: ListOrderedIcon,
            pressed: state.orderedList,
            onClick: () => editor.chain().focus().toggleOrderedList().run(),
          },
          {
            label: "Liên kết",
            icon: LinkIcon,
            pressed: state.link,
            onClick: () => (linkPanel ? closeLinkPanel() : openLinkPanel()),
          },
        ]}
      />
      {linkPanel ? (
        <div className="flex flex-wrap items-center gap-2 border-b border-line-200 px-2 py-2">
          <input
            // Focus the URL field as soon as the panel opens; the link
            // button kept the editor's selection through `mousedown`.
            ref={(node) => node?.focus()}
            type="url"
            aria-label="Địa chỉ liên kết"
            aria-invalid={linkPanel.error !== null}
            aria-describedby={linkPanel.error ? `${id}-link-error` : undefined}
            placeholder="https://"
            value={linkPanel.url}
            onChange={(event) => setLinkPanel({ url: event.target.value, error: null })}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                applyLink();
              } else if (event.key === "Escape") {
                event.preventDefault();
                closeLinkPanel();
              }
            }}
            className="min-w-0 flex-1 rounded-[10px] border border-line-200 px-2 py-1.5 text-[13px] text-ink-700 outline-none focus-visible:border-mint-400 aria-invalid:border-coral-400"
          />
          <button type="button" onClick={applyLink} className={linkActionClassName}>
            Áp dụng
          </button>
          {state.link ? (
            <button type="button" onClick={removeLink} className={linkActionClassName}>
              Bỏ liên kết
            </button>
          ) : null}
          {linkPanel.error ? (
            <p id={`${id}-link-error`} role="alert" className="w-full text-[12px] text-coral-400">
              {linkPanel.error}
            </p>
          ) : null}
        </div>
      ) : null}
      <EditorContent editor={editor} />
      {/* Not a live region: it changes on every keystroke and would talk
          over the text being typed. Crossing the limit is announced once.
          Hidden until the field has focus or the count nears the cap, so it
          doesn't clutter a form full of untouched fields. */}
      {showCounter ? (
        <p
          className={cn(
            "px-3 pb-2 text-right text-[11.5px]",
            overLimit ? "font-semibold text-coral-400" : "text-ink-400",
          )}
        >
          {state.count}/{DESCRIPTION_MAX_CHARS}
        </p>
      ) : null}
      <span role="status" className="sr-only">
        {overLimit ? `Mô tả vượt quá ${DESCRIPTION_MAX_CHARS} ký tự` : ""}
      </span>
    </div>
  );
}

const linkActionClassName =
  "rounded-[10px] px-2.5 py-1.5 text-[12.5px] font-bold text-mint-600 hover:bg-mint-50";

interface ToolbarItem {
  label: string;
  icon: LucideIcon;
  pressed: boolean;
  onClick: () => void;
}

/**
 * WAI-ARIA toolbar: one button sits in the tab order (roving tabindex),
 * the arrow keys move between the rest, Escape hands focus back to the text.
 */
function Toolbar({ editor, items }: { editor: Editor; items: ToolbarItem[] }) {
  const [focusIndex, setFocusIndex] = useState(0);
  const buttonsRef = useRef<(HTMLButtonElement | null)[]>([]);

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      editor.commands.focus();
      return;
    }
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    const index = buttonsRef.current.indexOf(document.activeElement as HTMLButtonElement);
    if (index === -1) return;
    event.preventDefault();
    const step = event.key === "ArrowRight" ? 1 : -1;
    const next = (index + step + items.length) % items.length;
    setFocusIndex(next);
    buttonsRef.current[next]?.focus();
  };

  return (
    <div
      role="toolbar"
      aria-label="Định dạng mô tả"
      onKeyDown={handleKeyDown}
      className="flex flex-wrap items-center gap-0.5 border-b border-line-200 px-1.5 py-1"
    >
      {items.map((item, index) => (
        <ToolbarButton
          key={item.label}
          ref={(node) => {
            buttonsRef.current[index] = node;
          }}
          item={item}
          tabIndex={index === focusIndex ? 0 : -1}
          onFocus={() => setFocusIndex(index)}
        />
      ))}
    </div>
  );
}

function ToolbarButton({
  item,
  tabIndex,
  onFocus,
  ref,
}: {
  item: ToolbarItem;
  tabIndex: number;
  onFocus: () => void;
  ref: (node: HTMLButtonElement | null) => void;
}) {
  const Icon = item.icon;
  return (
    <button
      ref={ref}
      type="button"
      aria-label={item.label}
      title={item.label}
      aria-pressed={item.pressed}
      tabIndex={tabIndex}
      onFocus={onFocus}
      // Keep the selection in the editor: a mousedown on the button would
      // otherwise blur ProseMirror before the command runs.
      onMouseDown={(event) => event.preventDefault()}
      onClick={item.onClick}
      className={cn(
        "inline-flex size-10 items-center justify-center rounded-md text-ink-500 hover:bg-cream-100 hover:text-ink-900",
        item.pressed && "bg-mint-50 text-mint-600",
      )}
    >
      <Icon aria-hidden className="size-4" />
    </button>
  );
}
