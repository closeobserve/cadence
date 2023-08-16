
access(all)
struct Character: Storable, Equatable, Comparable, Exportable, Importable {

    /// The byte array of the UTF-8 encoding
    access(all)
    let utf8: [UInt8]
}
