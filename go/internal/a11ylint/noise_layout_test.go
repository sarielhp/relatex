package a11ylint

import "testing"

// Panel layouts of figures are no data tables (decision 167): arrows joining pictures, a nested table
// as a panel, captions under a row of pictures, and pictures beside text named by a row of labels.
func TestLintSkipsPanelLayouts(t *testing.T) {
	expectNoTableItem(t, "arrows between pictures", `\begin{tabular}{ccc}
\includegraphics{a} & $\Rightarrow$ & \includegraphics{b} \\
&& $\bigr.$ \\
&& $\Downarrow$ \\
\includegraphics{d} & $\Leftarrow$ & \includegraphics{c} \\
\end{tabular}`)
	expectNoTableItem(t, "a nested table as a panel", `\begin{tabular}{cc}
(A) & \includegraphics{dual} \\
(B) & \begin{tabular}{c|c} & $p_1$ \\ \hline $d_1$ & 1 \\ $d_2$ & 0 \end{tabular} \\
\end{tabular}`)
	expectNoTableItem(t, "captions under pictures", `\begin{tabular}{c|c}
\includegraphics{a} & \includegraphics{b} \\
$f(x)=f(z)$ & $f(x)<f(z)$ \\
\end{tabular}`)
	expectNoTableItem(t, "panels named by labels", `\begin{tabular}{ccc}
\includegraphics{wspd} & {\begin{tabular}{c} $A_1$ \\ $A_2$ \end{tabular}} & $W = \{A_1, A_2\}$ \\
(i) & (ii) & (iii)
\end{tabular}`)
}

// A list of exercises set in columns, every cell starting with its label, is a layout; a table whose
// first column holds the labels keeps its item.
func TestLintSkipsListsInColumns(t *testing.T) {
	expectNoTableItem(t, "exercises in columns", `\begin{tabular}{lllll}
a) $\lim_{x\to c} \sqrt{x}$ & & b) $\lim_{x\to c} x^2$ & & c) $x$ \\
d) $\sin(1/x)$ & & e) $\sin(x)$ &
\end{tabular}`)
	expectTableItem(t, "labels in the first column", `\begin{tabular}{lll}
(a) & 3 & 4 \\
(b) & 5 & 6 \\
\end{tabular}`)
}

// Text beside pictures without labels beneath, and text rows after a caption row, keep the table.
func TestLintKeepsTablesBesidePanels(t *testing.T) {
	expectTableItem(t, "text beside pictures", `\begin{tabular}{cc}
\includegraphics{a} & data \\
\includegraphics{b} & more \\
(i) & (ii)
\end{tabular}`)
	expectTableItem(t, "rows after the captions", `\begin{tabular}{cc}
\includegraphics{a} & \includegraphics{b} \\
caption a & caption b \\
1 & 2 \\
\end{tabular}`)
	expectTableItem(t, "arrows in a data table", `\begin{tabular}{cc}
rule & $\Rightarrow$ \\
other & $\Leftarrow$ \\
\end{tabular}`)
}
