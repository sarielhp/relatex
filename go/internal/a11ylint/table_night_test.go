package a11ylint

import "testing"

// A \cline under a header row above the full rule that ends the header groups its columns: the
// header has two rows and is set apart (papers4_1014 of the night run's sample). The same table
// with no partial rule keeps the item.
func TestTableItemsSkipAGroupedHeader(t *testing.T) {
	grouped := `\begin{tabular}{c|cccccc}
\hline
\multirow{2}{*}{n}&\multicolumn{2}{c}{CPU Time(s)}&\multicolumn{2}{c}{Error1}&\multicolumn{2}{c}{Error2} \\
\cline{2-7}
&Alg. 2&Alg. 1&Alg. 2&Alg. 1&Alg. 2&Alg. 1\\
\hline
500&0.0018&0.0095&1.8677e-13&1.8947e-13&4.5030e-09&4.5030e-09\\
\hline
1000&0.0029&0.0324&3.2900e-12&1.8293e-13&6.3745e-09&6.3745e-09\\
\hline
2000&0.0081&0.1677&2.5098e-12&1.9489e-13&1.0236e-08&1.0236e-08\\
\hline
\end{tabular}`
	expectNoTableItem(t, "grouped header", grouped)
	plain := `\begin{tabular}{ccc}
\hline
n & CPU & Error \\
\hline
500&0.0018&0.0095\\
\hline
1000&0.0029&0.0324\\
\hline
2000&0.0081&0.1677\\
\hline
\end{tabular}`
	expectTableItem(t, "ruled alike", plain)
}

// A tabular in a TikZ node or in a caption is a legend, not a data table (papers3_0751).
func TestTableItemsSkipLegends(t *testing.T) {
	expectNoTableItem(t, "tikz node legend", `\begin{figure}
\begin{tikzpicture}
\begin{axis}\addplot table{data/32.dat};\label{dat_32}\end{axis}
\node[anchor=south west, draw=black, fill=white] (legend) at (plot.south west){\begin{tabular}{ll}
     \ref{dat_04}  & $\ell=4$ \\
     \ref{dat_08} & $\ell=8$\\
     \ref{dat_32} & $\ell=32$
\end{tabular}};
\end{tikzpicture}
\caption{Fragment size distribution.}
\end{figure}`)
	expectNoTableItem(t, "caption legend", `\begin{figure}
\includegraphics{a}
\caption{Marks: \begin{tabular}{ll} a & b \\ c & d \\ \end{tabular}}
\end{figure}`)
	expectTableItem(t, "after the drawing", `\begin{tikzpicture}
\node {x};
\end{tikzpicture}
\begin{tabular}{cc}
1 & 2 \\
3 & 4 \\
\end{tabular}`)
}
